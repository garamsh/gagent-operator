package constructor_test

import (
	"context"
	"errors"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	agentv1alpha1 "github.com/garamsh/gagent-operator/api/v1alpha1"
	"github.com/garamsh/gagent-operator/internal/garam"
	"github.com/garamsh/gagent-operator/internal/garam/constructor"
)

const (
	namespace   = "gagent-operator-system"
	image       = "registry.example/gagent:1.2.3"
	storageSize = "3Gi"

	sampleAgent = garam.GRN("grn:acme:default:agent:9f2ac1b40d8e7a35")
	otherAgent  = garam.GRN("grn:acme:default:agent:0a1b2c3d4e5f6071")
)

// sampleCredential is what garam answers the certificate route with, in the
// four parts that become files.
var sampleCredential = garam.AgentCredential{
	CertificatePEM: []byte("a certificate"),
	KeyPEM:         []byte("a key"),
	IssuerPEM:      []byte("the authority that signed it"),
	ServerRootPEM:  []byte("the root garam is verified against"),
	NotAfter:       time.Date(2026, 8, 26, 15, 1, 43, 0, time.UTC),
}

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatalf("register the core types: %v", err)
	}
	if err := agentv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("register the Agent type: %v", err)
	}
	return scheme
}

// newConstructor returns an Agent constructing through c, which stands in for
// the API server.
func newConstructor(t *testing.T, scheme *runtime.Scheme, c client.Client) *constructor.Agent {
	t.Helper()

	return constructor.NewAgent(c, scheme, namespace, image, resource.MustParse(storageSize))
}

// newClient substitutes the API server, which is the boundary this unit is
// written against. The status subresource is declared so that a status write
// reaches the object rather than being dropped.
func newClient(scheme *runtime.Scheme) client.WithWatch {
	return fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&agentv1alpha1.Agent{}).Build()
}

func TestConstructBuildsTheAgentFromTheOperatorsOwnConfiguration(t *testing.T) {
	g := NewWithT(t)
	scheme := newScheme(t)
	c := newClient(scheme)

	err := newConstructor(t, scheme, c).Construct(context.Background(), sampleAgent, sampleCredential)
	g.Expect(err).NotTo(HaveOccurred())

	constructed := &agentv1alpha1.Agent{}
	g.Expect(c.Get(context.Background(),
		client.ObjectKey{Namespace: namespace, Name: constructor.Name(sampleAgent)}, constructed)).To(Succeed())

	g.Expect(constructed.Spec.Image).To(Equal(image))
	g.Expect(constructed.Spec.StorageSize).To(Equal(resource.MustParse(storageSize)))
	g.Expect(constructed.Spec.CredentialsSecretName).To(Equal(constructor.Name(sampleAgent) + "-credentials"))
	g.Expect(constructed.Status.Agent).To(Equal(string(sampleAgent)))
}

// TestConstructPlacesEveryPartOfTheCredentialUnderItsOwnKey is what keeps the
// issuer and the server root apart. They are different certificates and an
// agent that verifies garam by the issuer cannot reach it at all, so each is
// written under a key naming which it is.
func TestConstructPlacesEveryPartOfTheCredentialUnderItsOwnKey(t *testing.T) {
	g := NewWithT(t)
	scheme := newScheme(t)
	c := newClient(scheme)

	err := newConstructor(t, scheme, c).Construct(context.Background(), sampleAgent, sampleCredential)
	g.Expect(err).NotTo(HaveOccurred())

	secret := &corev1.Secret{}
	g.Expect(c.Get(context.Background(), client.ObjectKey{
		Namespace: namespace, Name: constructor.Name(sampleAgent) + "-credentials"}, secret)).To(Succeed())

	g.Expect(secret.Data).To(Equal(map[string][]byte{
		constructor.CertificateKey: sampleCredential.CertificatePEM,
		constructor.KeyKey:         sampleCredential.KeyPEM,
		constructor.IssuerKey:      sampleCredential.IssuerPEM,
		constructor.ServerRootKey:  sampleCredential.ServerRootPEM,
	}))
}

// TestConstructOwnsTheCredentialByTheAgentItBelongsTo says a credential does not
// outlive the agent it was issued for: deleting the Agent is what removes the
// workload, and the key material goes with it.
func TestConstructOwnsTheCredentialByTheAgentItBelongsTo(t *testing.T) {
	g := NewWithT(t)
	scheme := newScheme(t)
	c := newClient(scheme)

	err := newConstructor(t, scheme, c).Construct(context.Background(), sampleAgent, sampleCredential)
	g.Expect(err).NotTo(HaveOccurred())

	secret := &corev1.Secret{}
	g.Expect(c.Get(context.Background(), client.ObjectKey{
		Namespace: namespace, Name: constructor.Name(sampleAgent) + "-credentials"}, secret)).To(Succeed())

	g.Expect(secret.OwnerReferences).To(HaveLen(1))
	g.Expect(secret.OwnerReferences[0].Kind).To(Equal("Agent"))
	g.Expect(secret.OwnerReferences[0].Name).To(Equal(constructor.Name(sampleAgent)))
}

func TestHasCredentialReportsWhatTheNamespaceCarries(t *testing.T) {
	g := NewWithT(t)
	scheme := newScheme(t)
	c := newClient(scheme)
	building := newConstructor(t, scheme, c)

	placed, err := building.HasCredential(context.Background(), sampleAgent)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(placed).To(BeFalse())

	g.Expect(building.Construct(context.Background(), sampleAgent, sampleCredential)).To(Succeed())

	placed, err = building.HasCredential(context.Background(), sampleAgent)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(placed).To(BeTrue())

	// A different agent, so that the answer above is the Secret this agent's
	// spec names and not any Secret in the namespace.
	placed, err = building.HasCredential(context.Background(), otherAgent)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(placed).To(BeFalse())
}

// TestConstructReplacesNoCredentialItAlreadyPlaced is what keeps this operator
// out of an agent's renewal. An agent renews its own certificate over the
// connection that one authenticated, so a second placement would replace a
// credential the agent may already have succeeded past.
func TestConstructReplacesNoCredentialItAlreadyPlaced(t *testing.T) {
	g := NewWithT(t)
	scheme := newScheme(t)
	c := newClient(scheme)
	building := newConstructor(t, scheme, c)

	g.Expect(building.Construct(context.Background(), sampleAgent, sampleCredential)).To(Succeed())

	renewed := sampleCredential
	renewed.CertificatePEM = []byte("a certificate the agent renewed for itself")
	renewed.KeyPEM = []byte("the key it was renewed with")
	g.Expect(building.Construct(context.Background(), sampleAgent, renewed)).To(Succeed())

	secret := &corev1.Secret{}
	g.Expect(c.Get(context.Background(), client.ObjectKey{
		Namespace: namespace, Name: constructor.Name(sampleAgent) + "-credentials"}, secret)).To(Succeed())
	g.Expect(secret.Data[constructor.CertificateKey]).To(Equal(sampleCredential.CertificatePEM))
}

// TestConstructAdoptsTheAgentItAlreadyBuilt says a second pass over a
// definition changes nothing: the Agent is created once and reported on once.
func TestConstructAdoptsTheAgentItAlreadyBuilt(t *testing.T) {
	g := NewWithT(t)
	scheme := newScheme(t)
	c := newClient(scheme)
	building := newConstructor(t, scheme, c)

	g.Expect(building.Construct(context.Background(), sampleAgent, sampleCredential)).To(Succeed())
	g.Expect(building.Construct(context.Background(), sampleAgent, sampleCredential)).To(Succeed())

	agents := &agentv1alpha1.AgentList{}
	g.Expect(c.List(context.Background(), agents, client.InNamespace(namespace))).To(Succeed())
	g.Expect(agents.Items).To(HaveLen(1))
	g.Expect(agents.Items[0].Status.Agent).To(Equal(string(sampleAgent)))
}

// TestHasCredentialReportsNothingHeldWhereOnlyTheAgentWasBuilt is the property
// a lost private key turns on. Construct creates the Agent before it places the
// credential, so a refused placement leaves an Agent standing with no key
// anywhere. Reported as constructed, that credential is gone: garam generated
// the key per certificate and keeps none, and nothing would ask for another.
// What is read is the Secret that holds the key, never the Agent beside it.
func TestHasCredentialReportsNothingHeldWhereOnlyTheAgentWasBuilt(t *testing.T) {
	g := NewWithT(t)
	scheme := newScheme(t)
	refusing := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&agentv1alpha1.Agent{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object,
				opts ...client.CreateOption) error {
				if _, secret := obj.(*corev1.Secret); secret {
					return errAPIRefusal
				}
				return c.Create(ctx, obj, opts...)
			},
		}).Build()
	building := newConstructor(t, scheme, refusing)

	g.Expect(building.Construct(context.Background(), sampleAgent, sampleCredential)).To(MatchError(errAPIRefusal))

	// The Agent stands, so that the answer below is the credential and not an
	// agent that was never built.
	constructed := &agentv1alpha1.Agent{}
	g.Expect(refusing.Get(context.Background(),
		client.ObjectKey{Namespace: namespace, Name: constructor.Name(sampleAgent)}, constructed)).To(Succeed())

	placed, err := building.HasCredential(context.Background(), sampleAgent)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(placed).To(BeFalse())
}

// TestConstructReportsWhatTheAPIRefused says a credential this fails to store
// is reported rather than swallowed: garam keeps no private key, so what does
// not reach the cluster is not obtainable again.
func TestConstructReportsWhatTheAPIRefused(t *testing.T) {
	g := NewWithT(t)
	scheme := newScheme(t)
	refusing := fake.NewClientBuilder().WithScheme(scheme).
		WithStatusSubresource(&agentv1alpha1.Agent{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object,
				opts ...client.CreateOption) error {
				if _, secret := obj.(*corev1.Secret); secret {
					return errAPIRefusal
				}
				return c.Create(ctx, obj, opts...)
			},
		}).Build()

	err := newConstructor(t, scheme, refusing).Construct(context.Background(), sampleAgent, sampleCredential)

	g.Expect(err).To(MatchError(errAPIRefusal))
	g.Expect(err).To(MatchError(ContainSubstring(string(sampleAgent))))
}

// TestNameIsTheDigestOfTheWholeGRN says the name reads no part of the GRN:
// what a GRN's segments mean is garam's, and a name cut out of one moves when
// that format does.
func TestNameIsTheDigestOfTheWholeGRN(t *testing.T) {
	g := NewWithT(t)

	name := constructor.Name(sampleAgent)

	g.Expect(name).To(HavePrefix("agent-"))
	g.Expect(name).NotTo(ContainSubstring("9f2ac1b40d8e7a35"))
	g.Expect(name).To(Equal(constructor.Name(sampleAgent)))
	g.Expect(name).NotTo(Equal(constructor.Name(otherAgent)))

	// The Pods of an Agent's workload carry its name with a suffix in a label,
	// so the API refuses one past 52 characters.
	g.Expect(len(name) + len("-credentials")).To(BeNumerically("<=", 52))
}

// errAPIRefusal stands in for whatever the API server answers a create it will
// not take.
var errAPIRefusal = errors.New("the API server refused the create")

// TestConstructNamesTheAgentInTheOperatorsOwnNamespace fixes where a
// constructed agent is built: the one namespace this operator was given, which
// is what keeps its power to create Secrets to that namespace alone.
func TestConstructNamesTheAgentInTheOperatorsOwnNamespace(t *testing.T) {
	g := NewWithT(t)
	scheme := newScheme(t)
	c := newClient(scheme)

	g.Expect(newConstructor(t, scheme, c).
		Construct(context.Background(), sampleAgent, sampleCredential)).To(Succeed())

	agents := &agentv1alpha1.AgentList{}
	g.Expect(c.List(context.Background(), agents)).To(Succeed())
	g.Expect(agents.Items).To(HaveLen(1))
	g.Expect(agents.Items[0].Namespace).To(Equal(namespace))

	secrets := &corev1.SecretList{}
	g.Expect(c.List(context.Background(), secrets)).To(Succeed())
	g.Expect(secrets.Items).To(HaveLen(1))
	g.Expect(secrets.Items[0].Namespace).To(Equal(namespace))
}
