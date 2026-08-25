package controller

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Agent workload", func() {
	It("builds the StatefulSet the Agent describes, and owns it", func() {
		name := "builds-its-workload"
		createSecret(credentialsSecretName(name))
		agent := newAgent(name)
		createAgent(agent)

		result, err := reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		workload := statefulSetFor(name)

		By("owning it, so that deleting the Agent takes it too")
		Expect(workload.OwnerReferences).To(HaveLen(1))
		Expect(workload.OwnerReferences[0].UID).To(Equal(agent.UID))
		Expect(workload.OwnerReferences[0].Controller).To(HaveValue(BeTrue()))

		By("running one replica of the Agent's image with the Agent's resources")
		Expect(workload.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)))
		Expect(workload.Spec.Template.Spec.Containers).To(HaveLen(1))
		container := workload.Spec.Template.Spec.Containers[0]
		Expect(container.Image).To(Equal(agent.Spec.Image))
		Expect(container.Resources.Requests.Memory().String()).To(Equal("256Mi"))

		By("putting the credential nowhere in the environment")
		Expect(container.Env).To(BeEmpty())
		Expect(container.EnvFrom).To(BeEmpty())

		By("claiming the Agent's storage size for the volume it keeps state on")
		Expect(workload.Spec.VolumeClaimTemplates).To(HaveLen(1))
		claim := workload.Spec.VolumeClaimTemplates[0]
		Expect(claim.Spec.Resources.Requests.Storage().String()).To(Equal("1Gi"))
		Expect(container.VolumeMounts).To(ContainElement(corev1.VolumeMount{
			Name:      claim.Name,
			MountPath: stateMountPath,
		}))
	})

	It("writes the StatefulSet no second time when the Agent has not changed", func() {
		name := "writes-once"
		createSecret(credentialsSecretName(name))
		createAgent(newAgent(name))

		_, err := reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())
		created := statefulSetFor(name).ResourceVersion

		_, err = reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(statefulSetFor(name).ResourceVersion).To(Equal(created))
	})

	It("brings the StatefulSet back to the image the Agent now names", func() {
		name := "follows-the-image"
		createSecret(credentialsSecretName(name))
		agent := newAgent(name)
		createAgent(agent)

		_, err := reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())
		created := statefulSetFor(name).ResourceVersion

		edited := readAgent(name)
		edited.Spec.Image = "example.com/gagent:v0.2.0"
		Expect(k8sClient.Update(ctx, edited)).To(Succeed())

		_, err = reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())

		workload := statefulSetFor(name)
		Expect(workload.Spec.Template.Spec.Containers[0].Image).To(Equal("example.com/gagent:v0.2.0"))
		Expect(workload.ResourceVersion).NotTo(Equal(created))
	})

	It("leaves the claim alone when the Agent's storage size changes, which a StatefulSet refuses", func() {
		name := "keeps-its-claim"
		createSecret(credentialsSecretName(name))
		agent := newAgent(name)
		createAgent(agent)

		_, err := reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())

		edited := readAgent(name)
		edited.Spec.StorageSize = resource.MustParse("2Gi")
		Expect(k8sClient.Update(ctx, edited)).To(Succeed())

		_, err = reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(statefulSetFor(name).Spec.VolumeClaimTemplates[0].Spec.Resources.Requests.Storage().String()).
			To(Equal("1Gi"))
	})

	It("puts the Agent's storage class on the claim, and leaves it unset when the Agent names none", func() {
		By("reconciling an Agent that names no storage class")
		unset := "cluster-default-class"
		createSecret(credentialsSecretName(unset))
		createAgent(newAgent(unset))

		_, err := reconcileAgent(unset)
		Expect(err).NotTo(HaveOccurred())
		Expect(statefulSetFor(unset).Spec.VolumeClaimTemplates[0].Spec.StorageClassName).To(BeNil())

		By("reconciling an Agent that differs only in naming one")
		named := "named-storage-class"
		createSecret(credentialsSecretName(named))
		agent := newAgent(named)
		agent.Spec.StorageClassName = ptr.To("fast")
		createAgent(agent)

		_, err = reconcileAgent(named)
		Expect(err).NotTo(HaveOccurred())
		Expect(statefulSetFor(named).Spec.VolumeClaimTemplates[0].Spec.StorageClassName).
			To(HaveValue(Equal("fast")))
	})

	It("delivers the credential from a volume the kubelet does not write, owned by the user that reads it", func() {
		name := "owns-its-credential"
		createSecret(credentialsSecretName(name))
		agent := newAgent(name)
		createAgent(agent)

		_, err := reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())

		pod := statefulSetFor(name).Spec.Template.Spec

		By("running the whole Pod as one user, which is what leaves the copy owned by its reader")
		Expect(pod.SecurityContext).NotTo(BeNil())
		Expect(pod.SecurityContext.RunAsUser).To(HaveValue(BeEquivalentTo(65532)))
		Expect(pod.SecurityContext.FSGroup).To(HaveValue(BeEquivalentTo(65532)))

		By("projecting the Secret group-readable, which is what the init container reads it by")
		projected := volumeNamed(pod, credentialsSecretVolumeName)
		Expect(projected.Secret).NotTo(BeNil())
		Expect(projected.Secret.SecretName).To(Equal(agent.Spec.CredentialsSecretName))
		Expect(projected.Secret.DefaultMode).To(HaveValue(BeEquivalentTo(0o440)))

		By("copying it into a memory-backed volume, so the copy never reaches the node's disk")
		copied := volumeNamed(pod, credentialsVolumeName)
		Expect(copied.EmptyDir).NotTo(BeNil())
		Expect(copied.EmptyDir.Medium).To(Equal(corev1.StorageMediumMemory))
		Expect(copied.EmptyDir.SizeLimit).NotTo(BeNil())

		By("making that copy before the agent starts, at a mode no group or other user can reach")
		Expect(pod.InitContainers).To(HaveLen(1))
		credentials := pod.InitContainers[0]
		Expect(credentials.Name).To(Equal(credentialsContainerName))
		Expect(credentials.Image).To(Equal(testCopyImage))
		script := strings.Join(credentials.Command, " ")
		Expect(script).To(ContainSubstring("install -m 0600"))
		Expect(script).To(ContainSubstring(credentialsSecretMountPath + "/*"))
		Expect(script).To(ContainSubstring(credentialsMountPath + "/"))
		Expect(credentials.VolumeMounts).To(ConsistOf(
			corev1.VolumeMount{Name: credentialsSecretVolumeName, MountPath: credentialsSecretMountPath, ReadOnly: true},
			corev1.VolumeMount{Name: credentialsVolumeName, MountPath: credentialsMountPath},
		))

		By("letting neither container name a user of its own, which would leave the copy owned by somebody else")
		Expect(credentials.SecurityContext).To(BeNil())
		Expect(pod.Containers).To(HaveLen(1))
		Expect(pod.Containers[0].SecurityContext).To(BeNil())

		By("giving the agent the copy and not the projection")
		Expect(pod.Containers[0].VolumeMounts).To(ConsistOf(
			corev1.VolumeMount{Name: credentialsVolumeName, MountPath: credentialsMountPath},
			corev1.VolumeMount{Name: stateVolumeName, MountPath: stateMountPath},
		))
	})
})

// volumeNamed returns the Pod's volume called name, and fails the spec where it
// carries none.
func volumeNamed(pod corev1.PodSpec, name string) corev1.Volume {
	GinkgoHelper()

	for _, volume := range pod.Volumes {
		if volume.Name == name {
			return volume
		}
	}

	Fail("the Pod carries no volume named " + name)

	return corev1.Volume{}
}
