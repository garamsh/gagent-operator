package garam

import (
	"context"
	"errors"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Poller reads this operator's definitions from garam on an interval, claims
// the ones it has not claimed yet, and constructs an agent for each one it
// holds a claim on.
//
// It runs beside the reconcilers rather than inside one: what wakes a poll is
// the clock, not an event on an Agent, and controller-runtime's shape for that
// is a Runnable. Added to a manager it joins the leader-election group
// (sigs.k8s.io/controller-runtime@v0.24.1:pkg/manager/runnable_group.go:99), so
// leader election is what keeps two processes from racing for one claim.
type Poller struct {
	client      *Client
	constructor Constructor
	interval    time.Duration
}

// NewPoller returns a Poller reading client's definitions every interval and
// constructing what it holds through constructor.
func NewPoller(client *Client, constructor Constructor, interval time.Duration) *Poller {
	return &Poller{client: client, constructor: constructor, interval: interval}
}

// Start polls until ctx is cancelled, which is what makes a Poller a
// manager.Runnable. It returns no error: an error here stops the manager, and
// a garam that cannot be reached is what the next poll is for.
func (p *Poller) Start(ctx context.Context) error {
	logf.FromContext(ctx).WithName("garam").Info("Polling garam for definitions", "interval", p.interval)
	wait.UntilWithContext(ctx, p.poll, p.interval)
	return nil
}

// poll reads the definitions once, claims each one garam does not already hold
// a claim for, and constructs each one this operator holds.
func (p *Poller) poll(ctx context.Context) {
	log := logf.FromContext(ctx).WithName("garam")

	definitions, err := p.client.ListDefinitions(ctx)
	if err != nil {
		log.Error(err, "Failed to read the definitions garam holds for this operator")
		return
	}
	log.V(1).Info("Read the definitions garam holds for this operator", "count", len(definitions))

	for _, definition := range definitions {
		if !definition.Claimed && !p.claim(ctx, definition.Agent) {
			continue
		}
		p.construct(ctx, definition.Agent)
	}
}

// claim binds this operator to agent and reports whether it now holds it.
func (p *Poller) claim(ctx context.Context, agent GRN) bool {
	log := logf.FromContext(ctx).WithName("garam")

	assignment, err := p.client.ClaimDefinition(ctx, agent)
	switch {
	case errors.Is(err, ErrClaimConflict):
		// Terminal rather than retryable: a definition is claimed once, so
		// the claim this operator holds is not one it can take again.
		log.Error(err, "Refused a claim garam already holds", "agent", agent)
		return false
	case err != nil:
		log.Error(err, "Failed to claim a definition", "agent", agent)
		return false
	}
	log.Info("Claimed a definition", "agent", assignment.Agent, "epoch", assignment.Epoch)
	return true
}

// construct builds the agent a claim admits this operator to, where the cluster
// does not already carry its credential.
//
// The certificate is asked for only here, where what comes back can be stored
// at once: garam generates the private key per certificate and stores none, so
// a credential that reaches nothing is gone. What recovers one is another
// certificate rather than another write, which the next pass asks for.
func (p *Poller) construct(ctx context.Context, agent GRN) {
	log := logf.FromContext(ctx).WithName("garam")

	placed, err := p.constructor.HasCredential(ctx, agent)
	if err != nil {
		log.Error(err, "Failed to read whether an agent's credential is placed", "agent", agent)
		return
	}
	if placed {
		return
	}

	credential, err := p.client.IssueAgentCertificate(ctx, agent)
	switch {
	case errors.Is(err, ErrAgentNotHeld):
		// garam answers the same for an agent held by another operator and for
		// one that does not exist, so there is nothing to tell apart and
		// nothing to retry against: the next read of the definitions is it.
		log.Error(err, "Constructed nothing: garam does not hold this agent for this operator", "agent", agent)
		return
	case err != nil:
		log.Error(err, "Failed to obtain an agent's credential", "agent", agent)
		return
	}

	if err := p.constructor.Construct(ctx, agent, credential); err != nil {
		log.Error(err, "Lost a certificate garam issued an agent: it exists nowhere else, and the next "+
			"pass asks for another rather than retrying this write", "agent", agent)
		return
	}
	log.Info("Constructed an agent", "agent", agent, "notAfter", credential.NotAfter)
}
