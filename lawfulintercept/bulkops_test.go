// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net/url"
	"testing"
	"time"

	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

// admfPeerCert issues the certificate the test ADMF presents on X1, binding its identifier to the
// ADMF role the way TS 103 221-1 annex G does. It is self-signed because nothing here
// verifies a chain — the server is handed the peer certificate directly, as the TLS layer
// would hand it one it had already validated — and what is under test is what the element
// does once the peer is authenticated.
func admfPeerCert(t *testing.T) *x509.Certificate {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	binding, err := url.Parse("urn:etsi:li:103221-1:cert-binding:ADMF:" + testADMFID)
	if err != nil {
		t.Fatalf("parse binding URN: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: testADMFID},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{binding},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	return cert
}

// bulkRequest builds one of the two bulk X1 messages: the one that stops every
// interception on this element, or the one that removes every destination it holds, sent by
// the test ADMF to the test element.
func bulkRequest(msgType string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<ns1:X1Request xmlns:ns1="http://uri.etsi.org/03221/X1/2017/10" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <ns1:x1RequestMessage xsi:type="ns1:` + msgType + `">
    <ns1:admfIdentifier>` + testADMFID + `</ns1:admfIdentifier>
    <ns1:neIdentifier>` + testNEID + `</ns1:neIdentifier>
    <ns1:messageTimestamp>2026-01-01T00:00:00.000000Z</ns1:messageTimestamp>
    <ns1:version>v1.6.1</ns1:version>
    <ns1:x1TransactionId>aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa</ns1:x1TransactionId>
  </ns1:x1RequestMessage>
</ns1:X1Request>`)
}

// TestBulkDeactivationFollowsConfiguration asserts that the agreement an operator states in
// this element's configuration is the one its X1 endpoint enforces.
//
// It goes through newX1Server — the constructor Init uses — rather than building an x1
// server of its own, because a second copy of the wiring is exactly where a configured
// policy stops being applied without anything failing.
//
// Both directions are covered: a stated `false` must not arrive as nothing, and an unstated
// value must not arrive as something.
func TestBulkDeactivationFollowsConfiguration(t *testing.T) {
	const neID = testNEID
	no := false

	cases := []struct {
		name        string
		configured  *bool
		wantRefusal bool
	}{
		{name: "agreed disabled", configured: &no, wantRefusal: true},
		{name: "no agreement in advance", configured: nil, wantRefusal: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := store.New()
			st.Activate(types.InterceptTask{
				XID:      "11111111-1111-4111-8111-111111111111",
				Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "262019876543210"}},
				Products: []types.ProductType{types.ProductIRI},
			})
			srv := newX1Server(st, Config{
				NEID:               neID,
				AdmfID:             testADMFID,
				DeactivateAllTasks: c.configured,
			}, &subsystem{store: st, neID: neID, ids: x2x3.NewIdentity(neID, amfInterceptionPoint)})

			resp, err := srv.Process(bulkRequest("DeactivateAllTasksRequest"),
				admfPeerCert(t))
			if err != nil {
				t.Fatalf("Process: %v", err)
			}
			m := resp.Messages[0]

			if !c.wantRefusal {
				if m.ErrorInformation != nil {
					t.Fatalf("want the standard's default — bulk deactivation performed — got error %d: %s",
						m.ErrorInformation.ErrorCode, m.ErrorInformation.ErrorDescription)
				}
				if st.Len() != 0 {
					t.Error("bulk deactivation was acknowledged and did not take effect")
				}

				return
			}

			if m.ErrorInformation == nil {
				t.Fatalf("the configured refusal did not reach the element: %+v", m)
			}
			if m.ErrorInformation.ErrorCode != 5010 {
				t.Errorf("code = %d, want the specification's 5010 for a disabled DeactivateAllTasks",
					m.ErrorInformation.ErrorCode)
			}
			if want := "DeactivateAllTasks message is not enabled"; m.ErrorInformation.ErrorDescription != want {
				t.Errorf("description = %q, want %q", m.ErrorInformation.ErrorDescription, want)
			}
			if st.Len() != 1 {
				t.Error("tasking was removed despite the refusal")
			}
		})
	}
}

// TestBulkRemovalFollowsConfiguration is the same assertion for the other switch, and it
// exists because without it the second of the two configuration fields is never observed
// to reach the element at all.
//
// The two are carried by one call taking both. So an element wired with the same field
// twice — the mistake that call is shaped to invite — reads the other field's unset value
// for bulk removal, performs the removal the operator disabled, and every other test here
// passes.
func TestBulkRemovalFollowsConfiguration(t *testing.T) {
	const neID = testNEID
	no := false

	cases := []struct {
		name        string
		configured  *bool
		wantRefusal bool
	}{
		{name: "agreed disabled", configured: &no, wantRefusal: true},
		{name: "no agreement in advance", configured: nil, wantRefusal: false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st := store.New()
			srv := newX1Server(st, Config{
				NEID:                  neID,
				AdmfID:                testADMFID,
				RemoveAllDestinations: c.configured,
			}, &subsystem{store: st, neID: neID, ids: x2x3.NewIdentity(neID, amfInterceptionPoint)})

			resp, err := srv.Process(bulkRequest("RemoveAllDestinationsRequest"),
				admfPeerCert(t))
			if err != nil {
				t.Fatalf("Process: %v", err)
			}
			m := resp.Messages[0]

			if !c.wantRefusal {
				if m.ErrorInformation != nil {
					t.Fatalf("want the standard's default — bulk removal performed — got error %d: %s",
						m.ErrorInformation.ErrorCode, m.ErrorInformation.ErrorDescription)
				}

				return
			}

			if m.ErrorInformation == nil {
				t.Fatalf("want the configured refusal of bulk removal, got %+v", m)
			}
			if m.ErrorInformation.ErrorCode != 8020 {
				t.Errorf("code = %d, want the specification's 8020 for a disabled RemoveAllDestinations",
					m.ErrorInformation.ErrorCode)
			}
		})
	}
}
