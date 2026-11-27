// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"encoding/xml"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// testTrust is the LI trust anchor every test in this package shares. It is also the
// element's own certificate, as liPKI writes it: a self-signed CA standing for
// both, with an IP SAN so a peer on the loopback presenting it is verified rather than merely
// trusted, and binding testNEID in the NE role by the TS 103 221-1 annex G URN, as a real
// element's certificate does for the ADMF that checks it. Sharing one is what lets the ADMF stub present a certificate the element verifies
// whichever of liPKI, loopbackPKI and newADMFStub a test happens to call first.
type testTrust struct {
	certPEM, keyPEM []byte
	cert            *x509.Certificate
	key             *ecdsa.PrivateKey
	pair            tls.Certificate
	pool            *x509.CertPool
}

var (
	trustOnce sync.Once
	trust     *testTrust
	trustErr  error
)

// liTrust returns the package's trust anchor, generating it on first use.
func liTrust(t *testing.T) *testTrust {
	t.Helper()

	trustOnce.Do(func() { trust, trustErr = newTestTrust() })
	if trustErr != nil {
		t.Fatal(trustErr)
	}

	return trust
}

func newTestTrust() (*testTrust, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	binding, err := url.Parse(certBindingURN("NE", testNEID))
	if err != nil {
		return nil, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: testNEID},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		URIs:                  []*url.URL{binding},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	tr := &testTrust{
		certPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		keyPEM:  pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}),
		cert:    cert,
		key:     key,
		pool:    x509.NewCertPool(),
	}
	tr.pool.AddCert(cert)
	if tr.pair, err = tls.X509KeyPair(tr.certPEM, tr.keyPEM); err != nil {
		return nil, err
	}

	return tr, nil
}

// admfCertificate issues the certificate an ADMF on the loopback presents: signed by the
// trust anchor, and binding admfID in the ADMF role by the TS 103 221-1 annex G URN, which
// is what the element's reporter checks in the handshake before it sends anything.
func (tr *testTrust) admfCertificate(t *testing.T, admfID string) tls.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := url.Parse(certBindingURN("ADMF", admfID))
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: admfID},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		URIs:         []*url.URL{binding},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tr.cert, &key.PublicKey, tr.key)
	if err != nil {
		t.Fatal(err)
	}

	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// liPKI writes the package's LI certificate, key and trust anchor, and returns their
// paths. Init loads credentials before it reads anything else, so a test that wants to
// reach the configuration checks has to get past mtls.Load first.
func liPKI(t *testing.T) (certPath, keyPath, caPath string) {
	t.Helper()

	tr := liTrust(t)
	dir := t.TempDir()
	certPath = filepath.Join(dir, "tls.crt")
	keyPath = filepath.Join(dir, "tls.key")
	caPath = filepath.Join(dir, "ca.crt")
	for path, body := range map[string][]byte{certPath: tr.certPEM, keyPath: tr.keyPEM, caPath: tr.certPEM} {
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	return certPath, keyPath, caPath
}

// certBindingURN is the TS 103 221-1 annex G certificate binding URN for identifier in role.
func certBindingURN(role, identifier string) string {
	return "urn:etsi:li:103221-1:cert-binding:" + role + ":" + identifier
}

// admfStub is an ADMF that receives NE issue reports and records their bodies. It is what the
// element's reporter talks to in production: an https endpoint presenting a certificate that
// binds testADMFID, requiring the element's own certificate, and answering each report with a
// response bound to it — so a report is committed as delivered, as it is against a real ADMF,
// rather than refused unsent or left unacknowledged.
//
// It checks what a real ADMF checks before acknowledging: that the element's certificate binds
// testNEID in the NE role (clause 8.2.4), and that the report names this ADMF and that NE. A
// report failing either is answered with an ErrorResponse, as an ADMF would, and fails the
// test when it ends, so a reporter sending the wrong identity cannot pass by being echoed.
type admfStub struct {
	srv *httptest.Server

	mu       sync.Mutex
	bodies   []string
	mismatch []string
}

func newADMFStub(t *testing.T) *admfStub {
	t.Helper()

	tr := liTrust(t)
	a := &admfStub{}
	// Registered before the server's Close, so it runs after it: every report has been
	// answered by the time the mismatches are read.
	t.Cleanup(func() {
		a.mu.Lock()
		defer a.mu.Unlock()
		for _, m := range a.mismatch {
			t.Errorf("the ADMF refused a report: %s", m)
		}
	})
	a.srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body) //nolint:errcheck // test handler

		problem := ""
		if r.TLS == nil || len(r.TLS.PeerCertificates) == 0 ||
			!bindsURN(r.TLS.PeerCertificates[0], certBindingURN("NE", testNEID)) {
			problem = "the client certificate does not bind " + testNEID + " in the NE role"
		}

		a.mu.Lock()
		a.bodies = append(a.bodies, string(body))
		a.mu.Unlock()

		answer, wrong, ok := admfAnswer(body)
		if problem == "" {
			problem = wrong
		}
		if problem != "" {
			a.mu.Lock()
			a.mismatch = append(a.mismatch, problem)
			a.mu.Unlock()
		}
		if ok {
			_, _ = w.Write([]byte(answer)) //nolint:errcheck // test handler
		}
	}))
	a.srv.TLS = &tls.Config{
		Certificates: []tls.Certificate{tr.admfCertificate(t, testADMFID)},
		ClientCAs:    tr.pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
	}
	a.srv.StartTLS()
	t.Cleanup(a.srv.Close)

	return a
}

// clientTLS is the TLS configuration of an element whose reporter a test builds by hand
// rather than through Init: the package's certificate, trusting the package's anchor.
func (a *admfStub) clientTLS() *tls.Config {
	return &tls.Config{
		Certificates: []tls.Certificate{trust.pair},
		RootCAs:      trust.pool,
		MinVersion:   tls.VersionTLS12,
	}
}

func (a *admfStub) received() []string {
	a.mu.Lock()
	defer a.mu.Unlock()

	return append([]string(nil), a.bodies...)
}

// bindsURN reports whether cert carries urn as a uniformResourceIdentifier subjectAltName.
func bindsURN(cert *x509.Certificate, urn string) bool {
	for _, u := range cert.URIs {
		if u != nil && u.String() == urn {
			return true
		}
	}
	return false
}

// admfAnswer is the response an ADMF gives a report: the request's own response type, its
// identifiers, version and transaction echoed, and an OK of AcknowledgedAndCompleted. A report
// naming another ADMF or NE is answered with an ErrorResponse instead, and wrong says why.
func admfAnswer(request []byte) (answer, wrong string, ok bool) {
	var in struct {
		Messages []struct {
			Type            string `xml:"http://www.w3.org/2001/XMLSchema-instance type,attr"`
			AdmfIdentifier  string `xml:"admfIdentifier"`
			NeIdentifier    string `xml:"neIdentifier"`
			Timestamp       string `xml:"messageTimestamp"`
			Version         string `xml:"version"`
			X1TransactionID string `xml:"x1TransactionId"`
		} `xml:"x1RequestMessage"`
	}
	if err := xml.Unmarshal(request, &in); err != nil || len(in.Messages) != 1 {
		return "", "", false
	}
	m := in.Messages[0]
	local := m.Type[strings.LastIndex(m.Type, ":")+1:]

	switch {
	case m.AdmfIdentifier != testADMFID:
		wrong = "it names ADMF " + m.AdmfIdentifier + ", not " + testADMFID
	case m.NeIdentifier != testNEID:
		wrong = "it names NE " + m.NeIdentifier + ", not " + testNEID
	}
	typ, outcome := strings.TrimSuffix(local, "Request")+"Response", `<x1:oK>AcknowledgedAndCompleted</x1:oK>`
	if wrong != "" {
		typ = "ErrorResponse"
		outcome = `<x1:requestMessageType>` + local + `</x1:requestMessageType>` +
			`<x1:errorInformation><x1:errorCode>1000</x1:errorCode>` +
			`<x1:errorDescription>` + wrong + `</x1:errorDescription></x1:errorInformation>`
	}

	return `<?xml version="1.0"?><x1:X1Response xmlns:x1="http://uri.etsi.org/03221/X1/2017/10" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` +
		`<x1:x1ResponseMessage xsi:type="x1:` + typ + `">` +
		`<x1:admfIdentifier>` + m.AdmfIdentifier + `</x1:admfIdentifier>` +
		`<x1:neIdentifier>` + m.NeIdentifier + `</x1:neIdentifier>` +
		`<x1:messageTimestamp>` + m.Timestamp + `</x1:messageTimestamp>` +
		`<x1:version>` + m.Version + `</x1:version>` +
		`<x1:x1TransactionId>` + m.X1TransactionID + `</x1:x1TransactionId>` +
		outcome +
		`</x1:x1ResponseMessage></x1:X1Response>`, wrong, true
}
