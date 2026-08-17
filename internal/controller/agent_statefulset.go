package controller

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	agentv1alpha1 "github.com/garamsh/gagent-operator/api/v1alpha1"
)

const (
	agentContainerName    = "agent"
	credentialsVolumeName = "credentials"
	stateVolumeName       = "state"

	credentialsMountPath = "/etc/gagent/credentials"
	stateMountPath       = "/var/lib/gagent"

	// credentialsFileMode keeps the mounted credential files readable by the
	// group the Pod carries and by nothing else. A Secret volume's files are
	// owned by root, so owner-only would leave them unreadable to an agent that
	// drops root — which is what an agent image should do. Tightening it back to
	// owner-only would not survive the kubelet anyway: where a Pod carries a
	// group, it ORs group-read into every file it writes into the volume.
	credentialsFileMode = 0o440

	// agentFSGroup is the group the kubelet gives every volume in the Pod and
	// adds to the supplementary groups of every process in it. It is not the uid
	// the image runs as and does not have to match it: a group is what carries
	// the access, so the operator never has to know which user the agent image
	// chose.
	agentFSGroup = 65532
)

// reconcileStatefulSet brings the StatefulSet an Agent describes into being, or
// brings an existing one back to what the Agent's spec says, and returns it as
// the cluster now holds it.
func (r *AgentReconciler) reconcileStatefulSet(ctx context.Context, agent *agentv1alpha1.Agent) (*appsv1.StatefulSet, error) {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: agent.Name, Namespace: agent.Namespace},
	}

	operation, err := controllerutil.CreateOrUpdate(ctx, r.Client, statefulSet, func() error {
		return applyAgent(agent, statefulSet, r.Scheme)
	})
	if err != nil {
		return nil, fmt.Errorf("create or update statefulset: %w", err)
	}

	if operation != controllerutil.OperationResultNone {
		logf.FromContext(ctx).Info("Reconciled the StatefulSet", "statefulSet", statefulSet.Name, "operation", operation)
	}

	return statefulSet, nil
}

// claimedStorageSize is the size of the volume the StatefulSet claims for the
// agent's state, and the zero quantity when it claims none. A claim template
// cannot be changed after creation, so this is what an Agent's storage size is
// worth comparing against.
func claimedStorageSize(statefulSet *appsv1.StatefulSet) resource.Quantity {
	for _, claim := range statefulSet.Spec.VolumeClaimTemplates {
		if claim.Name == stateVolumeName {
			return *claim.Spec.Resources.Requests.Storage()
		}
	}

	return resource.Quantity{}
}

// applyAgent writes the fields an Agent's spec decides onto statefulSet and
// leaves every other field as it found it, so that an unchanged Agent produces
// an unchanged object. The fields a StatefulSet refuses a change to are written
// at creation only.
func applyAgent(agent *agentv1alpha1.Agent, statefulSet *appsv1.StatefulSet, scheme *runtime.Scheme) error {
	if statefulSet.CreationTimestamp.IsZero() {
		labels := workloadLabels(agent)
		statefulSet.Labels = labels
		statefulSet.Spec.Selector = &metav1.LabelSelector{MatchLabels: labels}
		statefulSet.Spec.Template.Labels = labels
		statefulSet.Spec.VolumeClaimTemplates = []corev1.PersistentVolumeClaim{stateClaim(agent)}
	}

	// The agent's state is a single-writer store, so a second replica is never
	// correct.
	statefulSet.Spec.Replicas = ptr.To[int32](1)

	if statefulSet.Spec.Template.Spec.SecurityContext == nil {
		statefulSet.Spec.Template.Spec.SecurityContext = &corev1.PodSecurityContext{}
	}
	statefulSet.Spec.Template.Spec.SecurityContext.FSGroup = ptr.To[int64](agentFSGroup)

	statefulSet.Spec.Template.Spec.Volumes = []corev1.Volume{{
		Name: credentialsVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  agent.Spec.CredentialsSecretName,
				DefaultMode: ptr.To[int32](credentialsFileMode),
			},
		},
	}}

	container := containerNamed(&statefulSet.Spec.Template.Spec, agentContainerName)
	container.Image = agent.Spec.Image
	container.Resources = agent.Spec.Resources
	container.VolumeMounts = []corev1.VolumeMount{
		{Name: credentialsVolumeName, MountPath: credentialsMountPath, ReadOnly: true},
		{Name: stateVolumeName, MountPath: stateMountPath},
	}

	return controllerutil.SetControllerReference(agent, statefulSet, scheme)
}

// containerNamed returns the container called name, appending an empty one when
// the pod does not carry it yet. Writing through the returned pointer leaves the
// fields the API server defaulted on an existing container alone.
func containerNamed(pod *corev1.PodSpec, name string) *corev1.Container {
	for i := range pod.Containers {
		if pod.Containers[i].Name == name {
			return &pod.Containers[i]
		}
	}

	pod.Containers = append(pod.Containers, corev1.Container{Name: name})

	return &pod.Containers[len(pod.Containers)-1]
}

// stateClaim is the claim the agent's state volume is provisioned from.
func stateClaim(agent *agentv1alpha1.Agent) corev1.PersistentVolumeClaim {
	return corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{Name: stateVolumeName},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{corev1.ResourceStorage: agent.Spec.StorageSize},
			},
			// nil, never "": an empty string asks for no class at all, where an
			// Agent naming none asks for the cluster's default.
			StorageClassName: agent.Spec.StorageClassName,
		},
	}
}

// workloadLabels select the Pods of one Agent's workload. They sit in the
// StatefulSet's selector, which cannot change once it exists.
func workloadLabels(agent *agentv1alpha1.Agent) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     agentContainerName,
		"app.kubernetes.io/instance": agent.Name,
	}
}
