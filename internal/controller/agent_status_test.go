package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1alpha1 "github.com/garamsh/gagent-operator/api/v1alpha1"
)

// syncedCondition is the Synced condition an Agent of that name carries, and
// fails the spec when it carries none.
func syncedCondition(name string) *metav1.Condition {
	GinkgoHelper()

	agent := readAgent(name)
	synced := meta.FindStatusCondition(agent.Status.Conditions, agentv1alpha1.ConditionSynced)
	Expect(synced).NotTo(BeNil())

	return synced
}

var _ = Describe("Agent status", func() {
	It("reports the workload it reconciled, and no condition it does not set", func() {
		name := "reports-its-workload"
		createSecret(credentialsSecretName(name))
		createAgent(newAgent(name))

		_, err := reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())

		agent := readAgent(name)
		Expect(agent.Status.Conditions).To(HaveLen(1))
		Expect(agent.Status.ObservedGeneration).To(Equal(agent.Generation))

		synced := agent.Status.Conditions[0]
		Expect(synced.Type).To(Equal(agentv1alpha1.ConditionSynced))
		Expect(synced.Status).To(Equal(metav1.ConditionTrue))
		Expect(synced.Reason).To(Equal(agentv1alpha1.ReasonWorkloadReconciled))
		Expect(synced.ObservedGeneration).To(Equal(agent.Generation))
	})

	It("reports a credentials Secret that does not exist, and stops reporting it once it does", func() {
		name := "reports-missing-credentials"
		createAgent(newAgent(name))

		By("reconciling while the Secret is absent")
		_, err := reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())

		absent := syncedCondition(name)
		Expect(absent.Status).To(Equal(metav1.ConditionFalse))
		Expect(absent.Reason).To(Equal(agentv1alpha1.ReasonCredentialsSecretMissing))
		Expect(absent.Message).To(ContainSubstring(credentialsSecretName(name)))

		By("reconciling once the Secret exists")
		createSecret(credentialsSecretName(name))
		_, err = reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())

		arrived := syncedCondition(name)
		Expect(arrived.Status).To(Equal(metav1.ConditionTrue))
		Expect(arrived.Reason).To(Equal(agentv1alpha1.ReasonWorkloadReconciled))
	})

	It("reports a storage size the workload cannot be changed to, without failing", func() {
		name := "reports-immutable-storage"
		createSecret(credentialsSecretName(name))
		createAgent(newAgent(name))

		_, err := reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(syncedCondition(name).Status).To(Equal(metav1.ConditionTrue))

		By("asking for a size the claim template cannot take")
		edited := readAgent(name)
		edited.Spec.StorageSize = resource.MustParse("2Gi")
		Expect(k8sClient.Update(ctx, edited)).To(Succeed())

		_, err = reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())

		synced := syncedCondition(name)
		Expect(synced.Status).To(Equal(metav1.ConditionFalse))
		Expect(synced.Reason).To(Equal(agentv1alpha1.ReasonStorageSizeImmutable))
		Expect(synced.Message).To(ContainSubstring("2Gi"))
	})

	It("tracks the generation its status was computed from", func() {
		name := "tracks-its-generation"
		createSecret(credentialsSecretName(name))
		createAgent(newAgent(name))

		_, err := reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())
		first := readAgent(name)
		Expect(first.Status.ObservedGeneration).To(Equal(first.Generation))
		reported := first.Generation

		By("editing the spec, which leaves the status behind the generation")
		first.Spec.Image = "example.com/gagent:v0.2.0"
		Expect(k8sClient.Update(ctx, first)).To(Succeed())

		edited := readAgent(name)
		Expect(edited.Generation).To(BeNumerically(">", reported))
		Expect(edited.Status.ObservedGeneration).To(Equal(reported))

		By("reconciling the edit")
		_, err = reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())

		reconciled := readAgent(name)
		Expect(reconciled.Status.ObservedGeneration).To(Equal(reconciled.Generation))
	})

	It("writes the status no second time when nothing it observed changed", func() {
		name := "writes-its-status-once"
		createSecret(credentialsSecretName(name))
		createAgent(newAgent(name))

		_, err := reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())
		reported := readAgent(name).ResourceVersion

		_, err = reconcileAgent(name)
		Expect(err).NotTo(HaveOccurred())
		Expect(readAgent(name).ResourceVersion).To(Equal(reported))
	})
})
