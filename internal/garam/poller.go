package garam

import (
	"context"
	"errors"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// Poller reads this operator's definitions from garam on an interval and claims
// the ones it has not claimed yet.
//
// It runs beside the reconcilers rather than inside one: what wakes a poll is
// the clock, not an event on an Agent, and controller-runtime's shape for that
// is a Runnable. Added to a manager it joins the leader-election group
// (sigs.k8s.io/controller-runtime@v0.24.1:pkg/manager/runnable_group.go:99), so
// leader election is what keeps two processes from racing for one claim.
type Poller struct {
	client   *Client
	interval time.Duration
}

// NewPoller returns a Poller reading client's definitions every interval.
func NewPoller(client *Client, interval time.Duration) *Poller {
	return &Poller{client: client, interval: interval}
}

// Start polls until ctx is cancelled, which is what makes a Poller a
// manager.Runnable. It returns no error: an error here stops the manager, and
// a garam that cannot be reached is what the next poll is for.
func (p *Poller) Start(ctx context.Context) error {
	logf.FromContext(ctx).WithName("garam").Info("Polling garam for definitions", "interval", p.interval)
	wait.UntilWithContext(ctx, p.poll, p.interval)
	return nil
}

// poll reads the definitions once and claims each one garam does not already
// hold a claim for.
func (p *Poller) poll(ctx context.Context) {
	log := logf.FromContext(ctx).WithName("garam")

	definitions, err := p.client.ListDefinitions(ctx)
	if err != nil {
		log.Error(err, "Failed to read the definitions garam holds for this operator")
		return
	}
	log.V(1).Info("Read the definitions garam holds for this operator", "count", len(definitions))

	for _, definition := range definitions {
		if definition.Claimed {
			continue
		}
		assignment, err := p.client.ClaimDefinition(ctx, definition.Agent)
		switch {
		case errors.Is(err, ErrClaimConflict):
			// Terminal rather than retryable: a definition is claimed once, so
			// the claim this operator holds is not one it can take again.
			log.Error(err, "Refused a claim garam already holds", "agent", definition.Agent)
		case err != nil:
			log.Error(err, "Failed to claim a definition", "agent", definition.Agent)
		default:
			log.Info("Claimed a definition", "agent", assignment.Agent, "epoch", assignment.Epoch)
		}
	}
}
