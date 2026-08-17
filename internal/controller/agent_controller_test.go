package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	agentv1alpha1 "github.com/garamsh/gagent-operator/api/v1alpha1"
)

const agentNamespace = "default"

// newAgent returns an Agent the API server accepts, so that a spec differing
// from it in one field isolates that field.
func newAgent(name string) *agentv1alpha1.Agent {
	return &agentv1alpha1.Agent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: agentNamespace,
		},
		Spec: agentv1alpha1.AgentSpec{
			Image:                 "example.com/gagent:v0.1.0",
			CredentialsSecretName: "agent-credentials",
			StorageSize:           resource.MustParse("1Gi"),
		},
	}
}

var _ = Describe("Agent", func() {
	It("stores the spec it was created with, and refuses an image the CRD cannot accept", func() {
		By("creating an Agent the API server accepts")
		accepted := newAgent("stores-its-spec")
		Expect(k8sClient.Create(ctx, accepted)).To(Succeed())
		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, accepted)).To(Succeed())
		})

		By("reading the Agent back")
		readBack := &agentv1alpha1.Agent{}
		Expect(k8sClient.Get(ctx, client.ObjectKeyFromObject(accepted), readBack)).To(Succeed())
		Expect(readBack.Spec.Image).To(Equal("example.com/gagent:v0.1.0"))
		Expect(readBack.Spec.CredentialsSecretName).To(Equal("agent-credentials"))
		Expect(readBack.Spec.StorageSize.String()).To(Equal("1Gi"))

		By("creating the same Agent with an empty image")
		rejected := newAgent("empty-image")
		rejected.Spec.Image = ""
		err := k8sClient.Create(ctx, rejected)
		Expect(err).To(MatchError(ContainSubstring("spec.image")))
	})

	It("reconciles an Agent that is gone without returning an error", func() {
		reconciler := &AgentReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		result, err := reconciler.Reconcile(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: "never-created", Namespace: agentNamespace},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))
	})
})
