package controller

import (
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

		By("mounting the credentials Secret as files, and putting it nowhere in the environment")
		Expect(workload.Spec.Template.Spec.Volumes).To(HaveLen(1))
		credentials := workload.Spec.Template.Spec.Volumes[0]
		Expect(credentials.Secret.SecretName).To(Equal(agent.Spec.CredentialsSecretName))
		Expect(credentials.Secret.DefaultMode).To(HaveValue(BeEquivalentTo(0o400)))
		Expect(container.VolumeMounts).To(ContainElement(corev1.VolumeMount{
			Name:      credentials.Name,
			MountPath: credentialsMountPath,
			ReadOnly:  true,
		}))
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

		agent.Spec.Image = "example.com/gagent:v0.2.0"
		Expect(k8sClient.Update(ctx, agent)).To(Succeed())

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

		agent.Spec.StorageSize = resource.MustParse("2Gi")
		Expect(k8sClient.Update(ctx, agent)).To(Succeed())

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
})
