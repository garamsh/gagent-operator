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
	agentContainerName = "agent"

	// credentialsContainerName is the init container that copies the projected
	// credential into the volume the agent reads it from.
	credentialsContainerName = "credentials"

	credentialsVolumeName       = "credentials"
	credentialsSecretVolumeName = "credentials-secret"
	stateVolumeName             = "state"

	// credentialsMountPath holds the copy the agent reads. credentialsSecretMountPath
	// holds the projection the kubelet writes, and only the init container mounts it.
	credentialsMountPath       = "/run/gagent/credentials"
	credentialsSecretMountPath = "/etc/gagent/credentials"
	stateMountPath             = "/var/lib/gagent"

	// credentialsFileMode keeps the projected credential files readable by the
	// group the Pod carries and by nothing else. A Secret volume's files are
	// owned by root, so owner-only would leave them unreadable to the init
	// container that copies them, which does not run as root. Tightening it back
	// to owner-only would not survive the kubelet anyway: where a Pod carries a
	// group, it ORs group-read into every file it writes into the volume.
	credentialsFileMode = 0o440

	// credentialsCopyMode is the mode the copy carries. garam's reader refuses a
	// key file any group or other bit is set on.
	credentialsCopyMode = "0600"

	// agentFSGroup is the group the kubelet gives every volume in the Pod and
	// adds to the supplementary groups of each container's initial process. It is
	// what lets the init container open the projection, whose files the kubelet
	// writes root-owned at 0440. It buys no write: the destination emptyDir
	// arrives world-writable, so a measurement finding the copy writes without
	// the group has not shown the group unearned.
	agentFSGroup = 65532

	// agentRunAsUser is the user every container of the Pod runs as, so that the
	// copy the init container makes is owned by the process that reads it. The
	// operator names it rather than reading it off the image: an owner has to be
	// one value for the whole Pod, and no image can be asked what the others run
	// as.
	agentRunAsUser = 65532
)

// containerSecurityContext is what every container of the agent's Pod carries.
// It holds the two fields PodSecurity restricted asks of a container and nothing
// else: readOnlyRootFilesystem is no part of that standard, and naming a user
// here would take the Pod's own away from whichever container named it.
func containerSecurityContext() *corev1.SecurityContext {
	return &corev1.SecurityContext{
		AllowPrivilegeEscalation: ptr.To(false),
		Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
	}
}

// copyCredentialsCommand copies each projected credential file into the volume
// the agent reads, at a mode only its owner can reach. The glob skips the
// kubelet's dot-prefixed bookkeeping entries and names no key, so a Secret whose
// keys change does not change the workload.
func copyCredentialsCommand() []string {
	return []string{"/bin/sh", "-ec", fmt.Sprintf(
		"for f in %s/*; do install -m %s \"$f\" %s/; done",
		credentialsSecretMountPath, credentialsCopyMode, credentialsMountPath)}
}

// reconcileStatefulSet brings the StatefulSet an Agent describes into being, or
// brings an existing one back to what the Agent's spec says, and returns it as
// the cluster now holds it.
func (r *AgentReconciler) reconcileStatefulSet(ctx context.Context, agent *agentv1alpha1.Agent) (*appsv1.StatefulSet, error) {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: agent.Name, Namespace: agent.Namespace},
	}

	operation, err := controllerutil.CreateOrUpdate(ctx, r.Client, statefulSet, func() error {
		return applyAgent(agent, statefulSet, r.CopyImage, r.Scheme)
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
// at creation only. copyImage is the image the credential's init container runs
// and comes from this operator's own configuration rather than from the Agent.
func applyAgent(agent *agentv1alpha1.Agent, statefulSet *appsv1.StatefulSet, copyImage string, scheme *runtime.Scheme) error {
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
	podSecurity := statefulSet.Spec.Template.Spec.SecurityContext
	podSecurity.FSGroup = ptr.To[int64](agentFSGroup)
	podSecurity.RunAsUser = ptr.To[int64](agentRunAsUser)
	// Naming a user PodSecurity restricted would accept is not the same as
	// asserting one: it reads runAsNonRoot and refuses a Pod that leaves it
	// unset. Both fields sit here rather than on the containers because both are
	// one value for the whole Pod.
	podSecurity.RunAsNonRoot = ptr.To(true)
	podSecurity.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}

	statefulSet.Spec.Template.Spec.Volumes = []corev1.Volume{{
		Name: credentialsSecretVolumeName,
		VolumeSource: corev1.VolumeSource{
			Secret: &corev1.SecretVolumeSource{
				SecretName:  agent.Spec.CredentialsSecretName,
				DefaultMode: ptr.To[int32](credentialsFileMode),
			},
		},
	}, {
		Name: credentialsVolumeName,
		VolumeSource: corev1.VolumeSource{
			// Memory-backed, so the copy never reaches the node's disk, and
			// bounded, because a memory volume that names no limit is bounded
			// only by the node.
			EmptyDir: &corev1.EmptyDirVolumeSource{
				Medium:    corev1.StorageMediumMemory,
				SizeLimit: resource.NewQuantity(1<<20, resource.BinarySI),
			},
		},
	}}

	credentials := containerNamed(&statefulSet.Spec.Template.Spec.InitContainers, credentialsContainerName)
	credentials.Image = copyImage
	// Always on this operator's own ground: the copy image is the deployer's and
	// nothing here requires its tag to name one build, so a node's cache would
	// leave two agents copying a credential with different tools under one name.
	credentials.ImagePullPolicy = corev1.PullAlways
	credentials.Command = copyCredentialsCommand()
	credentials.SecurityContext = containerSecurityContext()
	credentials.VolumeMounts = []corev1.VolumeMount{
		{Name: credentialsSecretVolumeName, MountPath: credentialsSecretMountPath, ReadOnly: true},
		{Name: credentialsVolumeName, MountPath: credentialsMountPath},
	}

	container := containerNamed(&statefulSet.Spec.Template.Spec.Containers, agentContainerName)
	container.Image = agent.Spec.Image
	// gagent requires a consumer of its images to pull always, because the
	// repositories holding its bring-up builds are emptied when a release path
	// publishes: the default IfNotPresent turns a reference that stopped
	// resolving into a per-node stale cache
	// (gagent@04ed05a:docs/architecture/adr/0020-immutable-image-repositories.md).
	container.ImagePullPolicy = corev1.PullAlways
	container.Resources = agent.Spec.Resources
	container.SecurityContext = containerSecurityContext()
	// The copy is not mounted read-only: the rule it satisfies has the reader
	// owning the file, and garam's contract expects whatever refreshes a copy to
	// do so in the Pod that reads it.
	container.VolumeMounts = []corev1.VolumeMount{
		{Name: credentialsVolumeName, MountPath: credentialsMountPath},
		{Name: stateVolumeName, MountPath: stateMountPath},
	}

	return controllerutil.SetControllerReference(agent, statefulSet, scheme)
}

// containerNamed returns the container called name out of containers, appending
// an empty one when it does not carry it yet. Writing through the returned
// pointer leaves the fields the API server defaulted on an existing container
// alone.
func containerNamed(containers *[]corev1.Container, name string) *corev1.Container {
	for i := range *containers {
		if (*containers)[i].Name == name {
			return &(*containers)[i]
		}
	}

	*containers = append(*containers, corev1.Container{Name: name})

	return &(*containers)[len(*containers)-1]
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
