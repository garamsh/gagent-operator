package garam_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/garamsh/gagent-operator/internal/garam"
)

// enrollmentPath is the route an operator obtains its first certificate on
// (garam@b16a896:api/machine.yaml:147).
const enrollmentPath = "/enrollment"

// enrolledOperator is the identity garam's answer names. A token names the
// operator it was minted for and the request names none, so this is the answer's
// to say and never the request's.
const enrolledOperator = "grn:garam:default:operator:gagent"

// answerEnrollment answers an enrollment the way garam does: it reads the public
// key and the signature out of the request and nothing else, signs a certificate
// naming the operator the token named, and answers no private key because it
// generated none (garam@b16a896:api/machine.yaml:811-848).
func answerEnrollment(t *testing.T) http.HandlerFunc {
	t.Helper()

	return func(w http.ResponseWriter, r *http.Request) {
		presented := struct {
			Token                 string `json:"token"`
			CertificateRequestPEM string `json:"certificateRequestPem"`
		}{}
		if err := json.NewDecoder(r.Body).Decode(&presented); err != nil {
			t.Errorf("decode the enrollment presented: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprintf(w, `{"grn":%q,"certificatePem":%q,"issuerPem":%q,"serverRootPem":%q,"notAfter":%q}`,
			enrolledOperator, signRequest(t, []byte(presented.CertificateRequestPEM)),
			"an-issuer", "a-server-root", time.Now().Add(time.Hour).Format(time.RFC3339))
	}
}

// signRequest mints a certificate over the public key a request carries, which
// is what garam does with one: it holds no key of the subject and reads nothing
// else out of the request.
func signRequest(t *testing.T, requestPEM []byte) []byte {
	t.Helper()

	block, _ := pem.Decode(requestPEM)
	if block == nil {
		t.Fatalf("the enrollment presented no PEM block")
	}
	request, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil {
		t.Fatalf("read the certificate signing request presented: %v", err)
	}
	if err := request.CheckSignature(); err != nil {
		t.Fatalf("verify the certificate signing request presented: %v", err)
	}

	issuerKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate an issuing key: %v", err)
	}
	issuer := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "issuer"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	issuerDER, err := x509.CreateCertificate(rand.Reader, issuer, issuer, &issuerKey.PublicKey, issuerKey)
	if err != nil {
		t.Fatalf("mint an issuing certificate: %v", err)
	}
	signer, err := x509.ParseCertificate(issuerDER)
	if err != nil {
		t.Fatalf("read the issuing certificate back: %v", err)
	}

	leaf := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano() + 1),
		Subject:      pkix.Name{CommonName: enrolledOperator},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, leaf, signer, request.PublicKey, issuerKey)
	if err != nil {
		t.Fatalf("sign the key the request carried: %v", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// answerStatus answers every request with garam's error body under status.
func answerStatus(status int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"kind":"unauthenticated","message":"refused"}`))
	}
}

// newEnrollmentClient starts a listener serving handler at the enrollment route
// and returns a client reaching it the way an enrolling operator does: verifying
// the listener against the root the deployment supplied, presenting nothing.
func newEnrollmentClient(t *testing.T, handler http.HandlerFunc) (*garam.Client, *stubListener) {
	t.Helper()

	stub := newStubListener(t, handler)
	tlsConfig, err := garam.EnrollmentTLS(stub.trustFile)
	if err != nil {
		t.Fatalf("configure the connection to the stub listener: %v", err)
	}
	return garam.NewClient(stub.address(), tlsConfig), stub
}

// tamperWithSignature returns the request with the last byte of its DER flipped,
// which lands inside the signature. What it produces still parses as PKCS#10 —
// the test asserts that — so what refuses it is the signature check and not the
// encoding.
func tamperWithSignature(t *testing.T, request garam.CertificateRequest) garam.CertificateRequest {
	t.Helper()

	block, _ := pem.Decode(request.RequestPEM)
	if block == nil {
		t.Fatalf("the request built holds no PEM block")
	}
	tampered := append([]byte(nil), block.Bytes...)
	tampered[len(tampered)-1] ^= 0xff
	request.RequestPEM = pem.EncodeToMemory(&pem.Block{Type: block.Type, Bytes: tampered})
	return request
}

func TestNewCertificateRequestBuildsAPKCS10RequestOverAP256Key(t *testing.T) {
	g := NewWithT(t)

	request, err := garam.NewCertificateRequest()

	g.Expect(err).NotTo(HaveOccurred())
	block, _ := pem.Decode(request.RequestPEM)
	g.Expect(block).NotTo(BeNil())
	g.Expect(block.Type).To(Equal("CERTIFICATE REQUEST"))

	parsed, err := x509.ParseCertificateRequest(block.Bytes)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(parsed.CheckSignature()).To(Succeed())

	public, ok := parsed.PublicKey.(*ecdsa.PublicKey)
	g.Expect(ok).To(BeTrue())
	g.Expect(public.Curve).To(Equal(elliptic.P256()))

	// The key answers the request: a certificate signed over one and stored
	// beside the other authenticates nothing.
	keyBlock, _ := pem.Decode(request.KeyPEM)
	g.Expect(keyBlock).NotTo(BeNil())
	key, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(key.(*ecdsa.PrivateKey).PublicKey.Equal(public)).To(BeTrue())
}

// TestNewCertificateRequestNamesNoSubject holds this operator to what garam
// reads. A subject here is discarded rather than refused, so nothing fails when
// one is added — and what a certificate names is then written in two places,
// one of which garam ignores.
func TestNewCertificateRequestNamesNoSubject(t *testing.T) {
	g := NewWithT(t)

	request, err := garam.NewCertificateRequest()
	g.Expect(err).NotTo(HaveOccurred())

	block, _ := pem.Decode(request.RequestPEM)
	parsed, err := x509.ParseCertificateRequest(block.Bytes)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(parsed.Subject.String()).To(BeEmpty())
	g.Expect(parsed.DNSNames).To(BeEmpty())
	g.Expect(parsed.URIs).To(BeEmpty())
}

func TestEnrollReturnsTheCertificateGaramSigned(t *testing.T) {
	g := NewWithT(t)
	client, stub := newEnrollmentClient(t, answerEnrollment(t))
	request, err := garam.NewCertificateRequest()
	g.Expect(err).NotTo(HaveOccurred())

	enrolled, err := client.Enroll(context.Background(), "a-token", request)

	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(enrolled.Operator).To(Equal(garam.GRN(enrolledOperator)))
	g.Expect(string(enrolled.CertificatePEM)).To(ContainSubstring("BEGIN CERTIFICATE"))
	g.Expect(enrolled.NotAfter).To(BeTemporally(">", time.Now()))

	g.Expect(stub.requests()).To(HaveLen(1))
	g.Expect(stub.requests()[0].path).To(Equal(enrollmentPath))
	g.Expect(stub.requests()[0].body).To(ContainSubstring(`"token":"a-token"`))
	g.Expect(stub.requests()[0].body).To(ContainSubstring("BEGIN CERTIFICATE REQUEST"))
	// The one route on this listener a caller reaches with no certificate. An
	// operator that had one to present would not be enrolling.
	g.Expect(stub.requests()[0].client).To(BeEmpty())
}

// TestEnrollSendsNoRequestWhoseSignatureDoesNotVerify is the check that stands
// between a malformed request and a spent token: garam spends the token on the
// attempt, so a request it would refuse costs a registration.
//
// The control is the same request untampered, which is sent and answered. What
// separates the two is the signature alone: the tampered request is asserted to
// parse as PKCS#10, so nothing but the signature check can be refusing it.
func TestEnrollSendsNoRequestWhoseSignatureDoesNotVerify(t *testing.T) {
	g := NewWithT(t)
	client, stub := newEnrollmentClient(t, answerEnrollment(t))
	request, err := garam.NewCertificateRequest()
	g.Expect(err).NotTo(HaveOccurred())

	tampered := tamperWithSignature(t, request)
	block, _ := pem.Decode(tampered.RequestPEM)
	parsed, err := x509.ParseCertificateRequest(block.Bytes)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(parsed.CheckSignature()).NotTo(Succeed())

	_, err = client.Enroll(context.Background(), "a-token", tampered)

	g.Expect(err).To(HaveOccurred())
	g.Expect(stub.requests()).To(BeEmpty())

	// The control. The same call with the signature intact is sent, so what
	// refused the one above is the signature and not the call.
	_, err = client.Enroll(context.Background(), "a-token", request)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(stub.requests()).To(HaveLen(1))
}

// TestEnrollReportsARefusedTokenAsOneAnswer holds the client to what garam
// answers: spent, expired and never-minted are one refusal and a caller that
// told them apart would be inferring which.
func TestEnrollReportsARefusedTokenAsOneAnswer(t *testing.T) {
	g := NewWithT(t)
	client, _ := newEnrollmentClient(t, answerStatus(http.StatusUnauthorized))
	request, err := garam.NewCertificateRequest()
	g.Expect(err).NotTo(HaveOccurred())

	_, err = client.Enroll(context.Background(), "a-token", request)
	g.Expect(err).To(MatchError(garam.ErrTokenNotUsable))

	// The control: the other status this route answers is a request garam
	// refused, which is not a token to register again for.
	malformed, _ := newEnrollmentClient(t, answerStatus(http.StatusBadRequest))
	_, err = malformed.Enroll(context.Background(), "a-token", request)
	g.Expect(err).To(HaveOccurred())
	g.Expect(err).NotTo(MatchError(garam.ErrTokenNotUsable))
}

func TestEnrollRefusesAnAnswerCarryingNoCertificate(t *testing.T) {
	g := NewWithT(t)
	client, _ := newEnrollmentClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"grn":"grn:garam:default:operator:gagent"}`))
	})
	request, err := garam.NewCertificateRequest()
	g.Expect(err).NotTo(HaveOccurred())

	_, err = client.Enroll(context.Background(), "a-token", request)

	g.Expect(err).To(HaveOccurred())
}

// TestEnrollmentTLSVerifiesGaramAgainstTheRootItWasGiven is what keeps the token
// from being handed to whoever answered. An enrollment carries the token before
// anything comes back, so a connection that is not verified has already given it
// away — and the root the answer carries cannot be what verified the answer.
//
// The control is the same call verified against the listener's own root, which
// is answered.
func TestEnrollmentTLSVerifiesGaramAgainstTheRootItWasGiven(t *testing.T) {
	g := NewWithT(t)
	stub := newStubListener(t, answerEnrollment(t))
	request, err := garam.NewCertificateRequest()
	g.Expect(err).NotTo(HaveOccurred())

	elsewhere, _ := newCertificate(t, "another-garam")
	tlsConfig, err := garam.EnrollmentTLS(writeFile(t, t.TempDir(), "trust.pem", elsewhere))
	g.Expect(err).NotTo(HaveOccurred())

	_, err = garam.NewClient(stub.address(), tlsConfig).Enroll(context.Background(), "a-token", request)

	g.Expect(err).To(HaveOccurred())
	g.Expect(stub.requests()).To(BeEmpty())

	// The control: the same listener, verified against the root it presents.
	trusted, err := garam.EnrollmentTLS(stub.trustFile)
	g.Expect(err).NotTo(HaveOccurred())
	_, err = garam.NewClient(stub.address(), trusted).Enroll(context.Background(), "a-token", request)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(stub.requests()).To(HaveLen(1))
}

// startEnroller runs an Enroller until it returns or the test ends, and answers
// a channel closed when Start returned.
func startEnroller(t *testing.T, enroller *garam.Enroller) chan struct{} {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		_ = enroller.Start(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-stopped
	})
	return stopped
}

// newEnroller returns an Enroller reaching stub with the token and credential
// files a test placed, writing what it obtains through store.
func newEnroller(t *testing.T, stub *stubListener, store garam.CredentialStore, token, dir string) *garam.Enroller {
	t.Helper()

	tlsConfig, err := garam.EnrollmentTLS(stub.trustFile)
	if err != nil {
		t.Fatalf("configure the connection to the stub listener: %v", err)
	}
	tokenFile := dir + "/enrollment-token"
	if token != "" {
		tokenFile = writeFile(t, dir, "enrollment-token", []byte(token))
	}
	return garam.NewEnroller(garam.NewClient(stub.address(), tlsConfig), store, tokenFile,
		dir+"/certificate.pem", dir+"/key.pem")
}

// TestEnrollerStoresTheCertificateBesideTheKeyItGenerated is the whole of what
// enrollment is for. The certificate garam signed and the key this operator kept
// are a pair or they are nothing: garam generated no key and can answer no
// second copy of one.
func TestEnrollerStoresTheCertificateBesideTheKeyItGenerated(t *testing.T) {
	g := NewWithT(t)
	stub := newStubListener(t, answerEnrollment(t))
	store := newRecordingStore(nil)

	stopped := startEnroller(t, newEnroller(t, stub, store, "a-token", t.TempDir()))

	g.Eventually(stopped, time.Second*10).Should(BeClosed())
	g.Expect(store.written).To(HaveLen(1))
	stored := <-store.written
	_, err := tls.X509KeyPair(stored.CertificatePEM, stored.KeyPEM)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(stub.requests()).To(HaveLen(1))
	g.Expect(stub.requests()[0].path).To(Equal(enrollmentPath))
}

// TestEnrollerSpendsNothingWhereThisOperatorHoldsACertificate keeps a restart
// from spending a token nobody asked it to spend. A certificate this operator
// can read is one the renewer renews, and enrollment is what an operator with
// none asks for.
func TestEnrollerSpendsNothingWhereThisOperatorHoldsACertificate(t *testing.T) {
	g := NewWithT(t)
	stub := newStubListener(t, answerEnrollment(t))
	store := newRecordingStore(nil)
	dir := t.TempDir()
	writeIdentity(t, dir, "operator")

	stopped := startEnroller(t, newEnroller(t, stub, store, "a-token", dir))

	g.Eventually(stopped, time.Second*10).Should(BeClosed())
	g.Expect(stub.requests()).To(BeEmpty())
	g.Expect(store.written).To(BeEmpty())
}

// TestEnrollerWaitsWhereThereIsNoTokenToSpend is what makes a deployment that
// configured enrollment and has no token behave as one that did not. The flag is
// set wherever this operator runs, so this is the state most of them are in.
func TestEnrollerWaitsWhereThereIsNoTokenToSpend(t *testing.T) {
	for _, test := range []struct {
		name  string
		token string
	}{
		{name: "no file at all", token: ""},
		{name: "a file holding nothing", token: "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			g := NewWithT(t)
			stub := newStubListener(t, answerEnrollment(t))
			store := newRecordingStore(nil)

			stopped := startEnroller(t, newEnroller(t, stub, store, test.token, t.TempDir()))

			g.Consistently(stopped, time.Millisecond*200).ShouldNot(BeClosed())
			g.Expect(stub.requests()).To(BeEmpty())
			g.Expect(store.written).To(BeEmpty())
		})
	}
}

// TestEnrollerPresentsARefusedTokenOnce is the property a retry would break. A
// token is spent by the call that presents it, so a second call presents one
// that is gone and reports a state that has already moved.
func TestEnrollerPresentsARefusedTokenOnce(t *testing.T) {
	g := NewWithT(t)
	stub := newStubListener(t, answerStatus(http.StatusUnauthorized))
	store := newRecordingStore(nil)

	stopped := startEnroller(t, newEnroller(t, stub, store, "a-token", t.TempDir()))

	g.Eventually(stopped, time.Second*10).Should(BeClosed())
	g.Expect(stub.requests()).To(HaveLen(1))
	g.Expect(store.written).To(BeEmpty())
}

// TestEnrollerAsksForNothingElseWhereTheStoreRefusesTheAnswer is the loss this
// cannot recover from. garam signed a key it never held and answers no second
// copy, so a certificate the store would not take exists nowhere and the token
// that bought it is spent: what asks again is a person registering, and a second
// call here would present a token that is already gone.
func TestEnrollerAsksForNothingElseWhereTheStoreRefusesTheAnswer(t *testing.T) {
	g := NewWithT(t)
	stub := newStubListener(t, answerEnrollment(t))
	store := newRecordingStore(errors.New("the API server refused the patch"))

	stopped := startEnroller(t, newEnroller(t, stub, store, "a-token", t.TempDir()))

	g.Eventually(stopped, time.Second*10).Should(BeClosed())
	g.Expect(stub.requests()).To(HaveLen(1))
	g.Expect(store.written).To(HaveLen(1))
}
