package garam

import "context"

// Constructor builds the agents garam assigned this operator: the Agent the
// reconciler acts on, and the Secret its workload mounts the credential from.
//
// A definition says what an agent is and this operator says how one is built,
// so what a Constructor needs beyond the identity garam minted is its own
// configuration rather than anything the definition carries.
type Constructor interface {
	// HasCredential reports whether the credential an agent's workload mounts
	// is already placed. garam issues a private key once, so nothing asks for a
	// certificate it has nowhere to put.
	HasCredential(ctx context.Context, agent GRN) (bool, error)

	// Construct creates the Agent for a claimed definition, records the epoch
	// garam holds the agent at on it, and places credential where that Agent's
	// workload reads it. It is the step that makes the credential obtained:
	// garam keeps no private key and has already moved on, so one this fails to
	// store is recovered by asking for another certificate and never by
	// retrying the write.
	//
	// epoch is recorded because a report to garam carries it and nothing else
	// answers it later: a definition's claim reports whichever assignment
	// stands, and a claim is not repeatable.
	Construct(ctx context.Context, agent GRN, epoch int64, credential AgentCredential) error
}
