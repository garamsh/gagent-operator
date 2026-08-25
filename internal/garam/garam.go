// Package garam reads the agent definitions garam holds for this operator and
// claims them.
package garam

import "errors"

// GRN is a garam resource name. This operator carries one and never reads
// inside it: what its segments mean is garam's, and an identifier it minted is
// an identifier here.
type GRN string

// Definition is one agent an organization asked this operator to run.
type Definition struct {
	// Agent is the identity garam minted for the agent this definition
	// describes. No agent answers under it until the definition is claimed.
	Agent GRN

	// Values are what the operator constructs the agent from. garam stores the
	// keys and the strings and interprets neither, so what they mean is this
	// operator's contract rather than garam's.
	Values map[string]string

	// Claimed reports whether the definition is already claimed. garam admits no
	// claimant but the operator a definition names, so a claim it reports here
	// is this operator's own.
	Claimed bool
}

// Assignment binds an agent to the operator that runs it, and is what admits
// this operator to that agent's certificate route.
type Assignment struct {
	// Agent is the agent the assignment binds.
	Agent GRN

	// Operator is the operator it is bound to, which is the caller.
	Operator GRN

	// Epoch rises with every assignment and fences the operator it replaced.
	Epoch int64
}

// ErrClaimConflict is what a claim of an agent garam already holds a claim for
// answers. A definition is claimed once and a second claim is a conflict rather
// than an override, so retrying one changes nothing.
var ErrClaimConflict = errors.New("garam holds a claim on this agent already")
