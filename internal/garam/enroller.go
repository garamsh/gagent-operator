package garam

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"slices"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

// tokenInterval is how often an Enroller looks for the token it spends. An
// operator is deployed before it is registered, so the token is placed while
// this is already running and what this waits on is a person.
const tokenInterval = 10 * time.Second

// EnrollmentTLS returns the TLS configuration an enrollment is called over:
// garam is verified against trustFile, and no client certificate is presented
// because an operator enrolling holds none — this is the one route on that
// listener a caller reaches without one
// (garam@b16a896:internal/machine/listener.go:29).
//
// trustFile is the root this operator already trusts, and the garam server root
// the answer carries is never what verifies the call that carried it. An
// enrollment made without verifying garam hands the token to whoever answered,
// which is the one way the design can be used wrongly
// (garam@b16a896:docs/architecture/adr/0064-an-operator-enrolls-with-a-one-time-token-and-keeps-its-key-in-its-own-cluster.md).
func EnrollmentTLS(trustFile string) (*tls.Config, error) {
	roots, err := trustedRoots(trustFile)
	if err != nil {
		return nil, err
	}
	return &tls.Config{
		// garam's machine listener serves TLS 1.3 and nothing older
		// (garam@8f9dd9d:internal/machine/listener.go:26).
		MinVersion: tls.VersionTLS13,
		RootCAs:    roots,
	}, nil
}

// Enroller obtains the first certificate this operator authenticates with, by
// presenting a token a person minted by registering this operator, and stores it
// where the [Renewer] renews it from.
//
// It runs as a manager.Runnable and joins the leader-election group
// (sigs.k8s.io/controller-runtime@v0.24.1:pkg/manager/runnable_group.go:99),
// which is load bearing rather than tidy: a token is spent by the first call
// that presents it, so two processes enrolling one identity leave one of them
// holding nothing and a person registering again.
//
// It attempts once and never twice. A token is one attempt and not one
// certificate: a refused token is one to register again for, and a retry can
// only fail while reading as though the state were still open.
type Enroller struct {
	client *Client
	store  CredentialStore

	// tokenFile is where the token is read from. It is a path so that the token
	// reaches this process the way the credential does — as a file the kubelet
	// writes — and is read at the attempt rather than held.
	tokenFile string

	// certificateFile and keyFile are the credential this operator holds, which
	// is what says whether it needs one. They are the same files the handshake
	// reads.
	certificateFile string
	keyFile         string

	// trustFile is what verified the call this operator enrolls over, and is
	// what the garam server root in the answer is compared against. Nothing
	// here writes it.
	trustFile string
}

// NewEnroller returns an Enroller spending the token in tokenFile and writing
// what garam answers through store.
//
// client reaches garam presenting no certificate, which is what [EnrollmentTLS]
// answers with: an operator that has one does not enroll, and one that has none
// cannot present it.
func NewEnroller(client *Client, store CredentialStore, tokenFile, certificateFile, keyFile, trustFile string) *Enroller {
	return &Enroller{
		client:          client,
		store:           store,
		tokenFile:       tokenFile,
		certificateFile: certificateFile,
		keyFile:         keyFile,
		trustFile:       trustFile,
	}
}

// Start enrolls this operator once and returns, which is what makes an Enroller a
// manager.Runnable. It returns no error: an error here stops the manager, and a
// token that cannot be spent stops nothing else this operator does.
func (e *Enroller) Start(ctx context.Context) error {
	if e.attempt(ctx) {
		return nil
	}
	logf.FromContext(ctx).WithName("garam").Info(
		"Enrolling this operator when a token is placed", "file", e.tokenFile)
	_ = wait.PollUntilContextCancel(ctx, tokenInterval, false, func(ctx context.Context) (bool, error) {
		return e.attempt(ctx), nil
	})
	return nil
}

// attempt enrolls where there is a token to enroll with, and reports whether
// there is anything left to wait for.
//
// A certificate this operator can already read ends it: enrollment is what an
// operator with none asks for, and one that holds one would be spending a token
// to replace a certificate the [Renewer] renews. So does a token that has been
// presented, whatever came back.
func (e *Enroller) attempt(ctx context.Context) bool {
	log := logf.FromContext(ctx).WithName("garam")

	if _, err := operatorCertificate(e.certificateFile, e.keyFile); err == nil {
		log.Info("Enrolling nothing: this operator holds a certificate already")
		return true
	}

	token, err := os.ReadFile(e.tokenFile)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.V(1).Info("Read no enrollment token", "file", e.tokenFile, "reason", err.Error())
		}
		return false
	}
	// Trimmed because the token is a line a person copied out of a console, and
	// what carries it here is a file rather than a field.
	presented := strings.TrimSpace(string(token))
	if presented == "" {
		return false
	}

	e.enroll(ctx, presented)
	return true
}

// enroll spends the token: it generates a key, asks garam to sign a request over
// it, and stores the certificate that comes back beside the key it was signed
// for. Nothing it fails at is retried here.
func (e *Enroller) enroll(ctx context.Context, token string) {
	log := logf.FromContext(ctx).WithName("garam")

	request, err := NewCertificateRequest()
	if err != nil {
		log.Error(err, "Cannot enroll this operator. Nothing was presented and the token stands, "+
			"so what tries again is this operator restarted")
		return
	}

	enrolled, err := e.client.Enroll(ctx, token, request)
	switch {
	case errors.Is(err, ErrTokenNotUsable):
		log.Error(err, "Cannot enroll this operator. Spent, expired and never-minted are one answer "+
			"and nothing here tells them apart: register this operator again for another token")
		return
	case err != nil:
		log.Error(err, "Failed to enroll this operator. A token is spent by the call that presents it, "+
			"so this one is gone if the call reached garam: register this operator again for another")
		return
	}

	e.reportServerRoot(ctx, enrolled.ServerRootPEM)

	credential := Credential{CertificatePEM: enrolled.CertificatePEM, KeyPEM: request.KeyPEM}
	if err := e.store.ReplaceCredential(ctx, credential); err != nil {
		log.Error(err, "Lost the certificate this operator enrolled with. garam signed a key it never "+
			"held and can answer no second copy, and the token is spent: register this operator again")
		return
	}
	log.Info("Enrolled this operator. The kubelet carries the credential into the volume this "+
		"operator reads it from, and the calls before it lands fail at the handshake",
		"operator", enrolled.Operator, "notAfter", enrolled.NotAfter)
}

// reportServerRoot says once where the garam server root the answer carries is
// not one this operator verifies garam by. It stores nothing, changes nothing
// and fails nothing: what verifies garam is the deployment's to supply, and a
// root read out of the answer it authenticated verifies nothing.
//
// It is said at all because nothing else would say it. Nothing rewrites the
// trust file, so a root garam has moved on from is a file that goes on working
// until every handshake fails at once, and this answer is where the two are
// side by side.
func (e *Enroller) reportServerRoot(ctx context.Context, answered []byte) {
	log := logf.FromContext(ctx).WithName("garam")

	answers := certificateFingerprints(answered)
	if len(answers) == 0 {
		// A field this operator does not act on, so its absence is read at the
		// level the rest of the answer's detail is and not louder.
		log.V(1).Info("Compared no garam server root: the enrollment answered none")
		return
	}
	trusted := certificateFingerprints(readFileOrNothing(e.trustFile))
	for _, answer := range answers {
		if slices.Contains(trusted, answer) {
			continue
		}
		log.Info("The garam server root this enrollment answered is not one this operator verifies garam by. "+
			"Nothing here rewrites what verifies garam, so carrying a rotation into that file is the deployment's",
			"answered", answers, "trusted", trusted, "file", e.trustFile)
		return
	}
}

// certificateFingerprints is the SHA-256 of every certificate a PEM holds, over
// the DER as `openssl x509 -fingerprint -sha256` takes it, so that a line naming
// one names what a reader can compute from the file.
func certificateFingerprints(encoded []byte) []string {
	var fingerprints []string
	for rest := encoded; ; {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			return fingerprints
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		sum := sha256.Sum256(block.Bytes)
		fingerprints = append(fingerprints, hex.EncodeToString(sum[:]))
	}
}

// readFileOrNothing answers a file's contents, and nothing where it cannot be
// read. What reads it has already been given what it needs and is reporting.
func readFileOrNothing(name string) []byte {
	content, err := os.ReadFile(name)
	if err != nil {
		return nil
	}
	return content
}
