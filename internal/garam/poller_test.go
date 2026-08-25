package garam_test

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	. "github.com/onsi/gomega"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/garamsh/gagent-operator/internal/garam"
)

const (
	// pollInterval is short enough that a test sees several passes.
	pollInterval = 20 * time.Millisecond

	// pollTimeout bounds what a test waits for the poller to have done.
	pollTimeout = 5 * time.Second

	claimedAgent   = garam.GRN("grn:acme:default:agent:0a1b2c3d4e5f6071")
	unclaimedAgent = garam.GRN("grn:acme:default:agent:9f2ac1b40d8e7a35")
)

// runPoller polls stub until the test ends, and fails the test where Start
// answers an error rather than stopping quietly.
func runPoller(t *testing.T, stub *stubListener) {
	t.Helper()

	poller := garam.NewPoller(garam.NewClient(stub.address(), trustedBy(t, stub)), pollInterval)
	ctx, cancel := context.WithCancel(logf.IntoContext(context.Background(), testr.New(t)))
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		if err := poller.Start(ctx); err != nil {
			t.Errorf("the poller stopped with %v", err)
		}
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
}

// claims is the agents the stub was asked to claim, in the order it was asked.
func claims(stub *stubListener) []string {
	var agents []string
	for _, request := range stub.requests() {
		if request.method == http.MethodPost {
			agents = append(agents, strings.TrimSuffix(strings.TrimPrefix(request.path, "/definitions/"), "/claim"))
		}
	}
	return agents
}

func TestPollerClaimsTheDefinitionsGaramHoldsNoClaimFor(t *testing.T) {
	g := NewWithT(t)
	stub := newStubListener(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			answerJSON(w, http.StatusOK, `[
				{"agentGrn": "`+string(claimedAgent)+`", "values": {}, "claim": {"epoch": 1}},
				{"agentGrn": "`+string(unclaimedAgent)+`", "values": {"model": "haiku"}}
			]`)
			return
		}
		answerJSON(w, http.StatusCreated, `{"grn": "`+string(unclaimedAgent)+`",
			"operator": "grn:acme:default:operator:one", "epoch": 1}`)
	})

	runPoller(t, stub)

	g.Eventually(func() []string { return claims(stub) }, pollTimeout).Should(ContainElement(string(unclaimedAgent)))
	g.Consistently(func() []string { return claims(stub) }, 10*pollInterval).
		ShouldNot(ContainElement(string(claimedAgent)))
}

func TestPollerKeepsPollingAfterAReadThatFailed(t *testing.T) {
	g := NewWithT(t)
	var reads atomic.Int64
	stub := newStubListener(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			if reads.Add(1) == 1 {
				answerJSON(w, http.StatusServiceUnavailable, `{"kind": "unavailable", "message": "no"}`)
				return
			}
			answerJSON(w, http.StatusOK, `[{"agentGrn": "`+string(unclaimedAgent)+`", "values": {}}]`)
			return
		}
		answerJSON(w, http.StatusCreated, `{"grn": "`+string(unclaimedAgent)+`",
			"operator": "grn:acme:default:operator:one", "epoch": 1}`)
	})

	runPoller(t, stub)

	g.Eventually(func() []string { return claims(stub) }, pollTimeout).Should(ContainElement(string(unclaimedAgent)))
}

func TestPollerClaimsTheNextDefinitionAfterAClaimRefusedAsAConflict(t *testing.T) {
	g := NewWithT(t)
	refused := garam.GRN("grn:acme:default:agent:1111111111111111")
	stub := newStubListener(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			answerJSON(w, http.StatusOK, `[
				{"agentGrn": "`+string(refused)+`", "values": {}},
				{"agentGrn": "`+string(unclaimedAgent)+`", "values": {}}
			]`)
			return
		}
		if strings.Contains(r.URL.Path, string(refused)) {
			answerJSON(w, http.StatusConflict, `{"kind": "already_exists", "message": "definition already claimed"}`)
			return
		}
		answerJSON(w, http.StatusCreated, `{"grn": "`+string(unclaimedAgent)+`",
			"operator": "grn:acme:default:operator:one", "epoch": 1}`)
	})

	runPoller(t, stub)

	g.Eventually(func() []string { return claims(stub) }, pollTimeout).Should(ContainElement(string(unclaimedAgent)))
}

func TestPollerStopsWhenItsContextIsCancelled(t *testing.T) {
	g := NewWithT(t)
	stub := newStubListener(t, answerNoDefinitions)

	poller := garam.NewPoller(garam.NewClient(stub.address(), trustedBy(t, stub)), pollInterval)
	ctx, cancel := context.WithCancel(logf.IntoContext(context.Background(), testr.New(t)))
	stopped := make(chan error, 1)
	go func() { stopped <- poller.Start(ctx) }()

	g.Eventually(func() []stubRequest { return stub.requests() }, pollTimeout).ShouldNot(BeEmpty())
	cancel()
	g.Eventually(stopped, pollTimeout).Should(Receive(BeNil()))
}
