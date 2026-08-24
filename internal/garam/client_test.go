package garam_test

import (
	"context"
	"crypto/tls"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/garamsh/gagent-operator/internal/garam"
)

// sampleAgent is an agent GRN in the shape garam mints them.
const sampleAgent = garam.GRN("grn:acme:default:agent:9f2ac1b40d8e7a35")

// answerJSON writes status and body the way the machine listener does.
func answerJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(body))
}

func TestClientListDefinitionsReadsTheAgentValuesAndClaimGaramAnswers(t *testing.T) {
	g := NewWithT(t)
	stub := newStubListener(t, func(w http.ResponseWriter, _ *http.Request) {
		answerJSON(w, http.StatusOK, `[
			{"agentGrn": "`+string(sampleAgent)+`", "values": {"model": "haiku"}},
			{"agentGrn": "grn:acme:default:agent:0a1b2c3d4e5f6071", "values": {}, "claim": {"epoch": 3}}
		]`)
	})

	definitions, err := garam.NewClient(stub.address(), trustedBy(t, stub)).ListDefinitions(context.Background())
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(definitions).To(Equal([]garam.Definition{
		{Agent: sampleAgent, Values: map[string]string{"model": "haiku"}, Claimed: false},
		{Agent: "grn:acme:default:agent:0a1b2c3d4e5f6071", Values: map[string]string{}, Claimed: true},
	}))
	g.Expect(stub.requests()).To(HaveLen(1))
	g.Expect(stub.requests()[0].method).To(Equal(http.MethodGet))
	g.Expect(stub.requests()[0].path).To(Equal("/definitions"))
}

func TestClientListDefinitionsRefusesAnAnswerNamingNoAgent(t *testing.T) {
	g := NewWithT(t)
	answer := `[{"agentGrn": "` + string(sampleAgent) + `", "values": {}}]`
	stub := newStubListener(t, func(w http.ResponseWriter, _ *http.Request) {
		answerJSON(w, http.StatusOK, answer)
	})
	client := garam.NewClient(stub.address(), trustedBy(t, stub))

	// The answer that is read, so that the refusal below is the missing agent
	// and not the shape of the payload around it.
	_, err := client.ListDefinitions(context.Background())
	g.Expect(err).NotTo(HaveOccurred())

	answer = `[{"values": {}}]`
	_, err = client.ListDefinitions(context.Background())
	g.Expect(err).To(MatchError(ContainSubstring("names no agent")))
}

func TestClientClaimDefinitionBindsTheAgentItNames(t *testing.T) {
	g := NewWithT(t)
	stub := newStubListener(t, func(w http.ResponseWriter, _ *http.Request) {
		answerJSON(w, http.StatusCreated, `{
			"grn": "`+string(sampleAgent)+`",
			"operator": "grn:acme:default:operator:one",
			"epoch": 1
		}`)
	})

	assignment, err := garam.NewClient(stub.address(), trustedBy(t, stub)).
		ClaimDefinition(context.Background(), sampleAgent)
	g.Expect(err).NotTo(HaveOccurred())

	g.Expect(assignment).To(Equal(garam.Assignment{
		Agent:    sampleAgent,
		Operator: "grn:acme:default:operator:one",
		Epoch:    1,
	}))
	g.Expect(stub.requests()).To(HaveLen(1))
	g.Expect(stub.requests()[0].method).To(Equal(http.MethodPost))
	g.Expect(stub.requests()[0].path).To(Equal("/definitions/" + string(sampleAgent) + "/claim"))
}

func TestClientClaimDefinitionReportsAConflictAndNoOtherRefusalAsOne(t *testing.T) {
	g := NewWithT(t)
	status := http.StatusConflict
	stub := newStubListener(t, func(w http.ResponseWriter, _ *http.Request) {
		answerJSON(w, status, `{"kind": "already_exists", "message": "definition already claimed"}`)
	})
	client := garam.NewClient(stub.address(), trustedBy(t, stub))

	_, err := client.ClaimDefinition(context.Background(), sampleAgent)
	g.Expect(err).To(MatchError(garam.ErrClaimConflict))

	// A refusal that is not a conflict, so that the match above is the status
	// and not every refusal reading as one.
	status = http.StatusServiceUnavailable
	_, err = client.ClaimDefinition(context.Background(), sampleAgent)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).NotTo(MatchError(garam.ErrClaimConflict))
}

// trustedBy is the TLS configuration verifying stub and nothing else.
func trustedBy(t *testing.T, stub *stubListener) *tls.Config {
	t.Helper()

	certificateFile, keyFile := writeIdentity(t, t.TempDir(), "operator")
	tlsConfig, err := garam.MutualTLS(certificateFile, keyFile, stub.trustFile)
	if err != nil {
		t.Fatalf("configure mutual TLS against the stub listener: %v", err)
	}
	return tlsConfig
}
