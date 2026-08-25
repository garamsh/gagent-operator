// Package garam reads the agent definitions garam holds for this operator and
// claims them.
package garam

import (
	"errors"
	"time"
)

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

// AgentCredential is what garam issues an agent: the certificate it
// authenticates as, the private key it was issued with, the authority that
// signed it, and the root it verifies garam's listener by.
//
// The last two are different certificates and are not interchangeable. garam is
// verified against ServerRootPEM and against nothing else; IssuerPEM is the
// authority that signed this certificate, which signs no listener
// (garam@5130ca9:api/machine.yaml:467-479).
type AgentCredential struct {
	// CertificatePEM is the certificate, PEM-encoded.
	CertificatePEM []byte

	// KeyPEM is the private key it was issued with, PEM-encoded. garam stores
	// none, so this arrives once and a holder that loses it asks for another
	// certificate.
	KeyPEM []byte

	// IssuerPEM is the authority that signed the certificate, PEM-encoded.
	IssuerPEM []byte

	// ServerRootPEM is the root garam's listener is verified against,
	// PEM-encoded.
	ServerRootPEM []byte

	// NotAfter is when the certificate stops being valid. garam answers it, so
	// nothing here parses a PEM to learn it.
	NotAfter time.Time
}

// ErrAgentNotHeld is what garam answers an agent this operator does not hold at
// the current epoch. It answers the same for an agent assigned to another
// operator, for one this operator was replaced on, and for one that does not
// exist, so the three cannot be told apart: this reports "not mine right now"
// and the next read of the definitions is the whole of the response.
var ErrAgentNotHeld = errors.New("garam holds this agent for another operator, or holds no such agent")

// ErrClaimConflict is what a claim of an agent garam already holds a claim for
// answers. A definition is claimed once and a second claim is a conflict rather
// than an override, so retrying one changes nothing.
var ErrClaimConflict = errors.New("garam holds a claim on this agent already")

// Credential is what this operator authenticates to garam as: the certificate
// and the private key it was issued with. garam stores no private key, so a
// renewal answers with one once and there is no second chance to read it.
type Credential struct {
	// CertificatePEM is the certificate, PEM-encoded.
	CertificatePEM []byte

	// KeyPEM is the private key it was issued with, PEM-encoded.
	KeyPEM []byte
}

// ErrRenewalTooEarly is what garam answers a renewal it admits no sooner. The
// window opens once two thirds of the presented certificate's own validity has
// passed, so this reports a clock rather than a fault and the next attempt is
// the whole of the remedy.
var ErrRenewalTooEarly = errors.New("garam admits no renewal of this certificate yet")

// ErrCredentialSuperseded is what garam answers a renewal of a certificate it
// has already replaced. garam renews the certificate it last issued this
// operator and no other, so the one presented is now outside that lineage: it
// authenticates until it expires and no retry recovers it, because only a mint
// out of band takes the lineage back.
var ErrCredentialSuperseded = errors.New("garam has issued a newer certificate for this operator already")
