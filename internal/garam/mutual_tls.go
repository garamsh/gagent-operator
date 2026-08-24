package garam

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
)

// MutualTLS returns the TLS configuration a [Client] reaches garam's machine
// listener with: this operator presents the certificate in certificateFile and
// keyFile, and verifies the listener against trustFile alone.
//
// The certificate is read at each handshake rather than held. garam publishes
// no revocation list and runs no responder, so expiry is the whole of its
// revocation and lifetimes are short; a configuration holding one certificate
// stops authenticating on a schedule.
//
// trustFile is a path so that what this operator trusts is the deployment's to
// supply. It is not the organization issuer an operator's own certificate
// arrives with: that authority signs this operator, and this one signs garam.
func MutualTLS(certificateFile, keyFile, trustFile string) (*tls.Config, error) {
	trusted, err := os.ReadFile(trustFile)
	if err != nil {
		return nil, fmt.Errorf("read the file garam is verified against: %w", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(trusted) {
		return nil, fmt.Errorf("%s holds no PEM certificate for garam to be verified against", trustFile)
	}
	// Read here as well as in the callback, so that a certificate this process
	// cannot read fails the operator at startup rather than every poll after it.
	if _, err := operatorCertificate(certificateFile, keyFile); err != nil {
		return nil, err
	}
	return &tls.Config{
		// garam's machine listener serves TLS 1.3 and nothing older
		// (garam@8f9dd9d:internal/machine/listener.go:26).
		MinVersion: tls.VersionTLS13,
		RootCAs:    roots,
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			return operatorCertificate(certificateFile, keyFile)
		},
	}, nil
}

// operatorCertificate reads the certificate this operator authenticates to
// garam as.
func operatorCertificate(certificateFile, keyFile string) (*tls.Certificate, error) {
	certificate, err := tls.LoadX509KeyPair(certificateFile, keyFile)
	if err != nil {
		return nil, fmt.Errorf("read the certificate this operator authenticates to garam with: %w", err)
	}
	return &certificate, nil
}
