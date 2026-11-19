// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
)

// oidUID is the UID relative distinguished name (RFC 4519 section 2.39), which is
// where TS 103 221-1 clause 8.2.4 puts the peer's identifier.
var oidUID = asn1.ObjectIdentifier{0, 9, 2342, 19200300, 100, 1, 1}

// admfCert is the certificate the responsible ADMF presents, bound to the
// identifier the request bodies below assert.
func admfCert(t *testing.T, identifier string) *x509.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "li-peer",
			ExtraNames: []pkix.AttributeTypeAndValue{{Type: oidUID, Value: identifier}},
		},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter:  time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("creating certificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parsing certificate: %v", err)
	}

	return cert
}

// activateWithTargets renders an ActivateTaskRequest naming the given target
// identifier elements verbatim.
func activateWithTargets(targets string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<ns1:X1Request xmlns:ns1="http://uri.etsi.org/03221/X1/2017/10" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <ns1:x1RequestMessage xsi:type="ns1:ActivateTaskRequest">
    <ns1:admfIdentifier>admf-1</ns1:admfIdentifier>
    <ns1:neIdentifier>amf-1</ns1:neIdentifier>
    <ns1:messageTimestamp>2026-08-16T18:46:21.247432Z</ns1:messageTimestamp>
    <ns1:version>v1.6.1</ns1:version>
    <ns1:x1TransactionId>3741800e-971b-4aa9-85f4-466d2b1adc7f</ns1:x1TransactionId>
    <ns1:taskDetails>
      <ns1:xId>50b93d1e-1b53-4d63-aacb-e4d99811bc0b</ns1:xId>
      <ns1:targetIdentifiers>` + targets + `</ns1:targetIdentifiers>
      <ns1:deliveryType>X2Only</ns1:deliveryType>
      <ns1:listOfDIDs>
        <ns1:dId>7d1c2f60-8a4e-4a1e-9f3b-2c5d6e7f8091</ns1:dId>
      </ns1:listOfDIDs>
    </ns1:taskDetails>
  </ns1:x1RequestMessage>
</ns1:X1Request>`
}

const (
	supiTarget = `<ns1:targetIdentifier><ns1:supiimsi>262019876543210</ns1:supiimsi></ns1:targetIdentifier>`
	ipTarget   = `<ns1:targetIdentifier><ns1:ipv4Address>10.45.0.2</ns1:ipv4Address></ns1:targetIdentifier>`
	// otherSUPITarget is a second subscriber: no UE presents two SUPIs.
	otherSUPITarget = `<ns1:targetIdentifier><ns1:supiimsi>262019876543211</ns1:supiimsi></ns1:targetIdentifier>`
	taskXID         = types.XID("50b93d1e-1b53-4d63-aacb-e4d99811bc0b")
)

// TestX1ServerRefusesTaskingItCannotAct drives the X1 server this element
// actually runs, which is the point: a decision never wired in refuses nothing,
// and a test that calls canApply directly passes with the option unregistered.
//
// This element matches subjects by subscriber identity alone, and every criterion of a
// task must match (TS 103 221-1 table 6.2.1.2-1). A warrant naming a UE address — alone
// or beside a subscriber identity — matches nothing here at every moment, and so does
// one naming two SUPIs; acknowledging either tells the ADMF an interception is running
// that cannot be — indistinguishable, from the agency's side, from a tasked subject who
// did nothing.
func TestX1ServerRefusesTaskingItCannotAct(t *testing.T) {
	for _, tc := range []struct {
		name     string
		targets  string
		wantHeld bool
	}{
		{"only an address this element never resolves", ipTarget, false},
		{"a subscriber identity", supiTarget, true},
		{"a subscriber identity and an address", supiTarget + ipTarget, false},
		{"two subscriber identities", supiTarget + otherSUPITarget, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st := store.New()
			// The destination the fixture names is declared, because a task naming one
			// this element cannot resolve is refused — and what this test is about is
			// whether the element can act on the *targets*.
			srv := newX1Server(st, Config{
				NEID: testNEID, AdmfID: testADMFID,
				Destinations: []Destination{{
					DID: "7d1c2f60-8a4e-4a1e-9f3b-2c5d6e7f8091", DeliveryType: "X2Only",
					Address: testAgencyAddr,
				}},
			}, &subsystem{store: st})

			resp, err := srv.Process([]byte(activateWithTargets(tc.targets)), admfCert(t, testADMFID))
			if err != nil {
				t.Fatalf("Process: %v", err)
			}
			if len(resp.Messages) != 1 {
				t.Fatalf("got %d response messages, want 1", len(resp.Messages))
			}

			_, held := st.Get(taskXID)
			if held != tc.wantHeld {
				t.Errorf("task held = %v, want %v", held, tc.wantHeld)
			}

			errInfo := resp.Messages[0].ErrorInformation
			if tc.wantHeld {
				if errInfo != nil {
					t.Errorf("warrant refused with %q — this element can act on one of the "+
						"identifiers it names, and refusing declines interception it can perform",
						errInfo.ErrorDescription)
				}

				return
			}
			if errInfo == nil {
				t.Fatal("a warrant this element cannot act on was acknowledged; the ADMF now " +
					"believes an interception is running that never can be")
			}
			if !strings.Contains(errInfo.ErrorDescription, "every criterion of a task must match") {
				t.Errorf("refusal reason = %q, want it to say why — every criterion must match — "+
					"so a provisioning function can tell this from a fault",
					errInfo.ErrorDescription)
			}
		})
	}
}

// TestATaskRequiringOnlyContentIsRefused is the product half of "any element refuses tasking
// it cannot act on".
//
// This element is an IRI-POI: it produces xIRI and nothing else. Were `canApply` to test only
// the identifier kinds, an `X3Only` warrant naming a valid SUPI would be acknowledged, stored,
// reported as active in an interrogation, and then skipped by every delivery path for not
// requesting IRI. The provisioning function would be told an interception is running at an
// element that can never produce anything for it — indistinguishable, from the agency's side,
// from a tasked subject who did nothing.
//
// `X2andX3` is accepted, and the distinction is the point: a warrant provisioned to several
// elements legitimately names products only some of them produce, so refusing that would
// refuse the ordinary multi-element warrant.
func TestATaskRequiringOnlyContentIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name     string
		products []types.ProductType
		refuse   bool
	}{
		{"X3Only", []types.ProductType{types.ProductCC}, true},
		{"X2Only", []types.ProductType{types.ProductIRI}, false},
		{"X2andX3", []types.ProductType{types.ProductIRI, types.ProductCC}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := canApply(types.InterceptTask{
				XID:      taskXID,
				Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "262019876543210"}},
				Products: tc.products,
			})

			if tc.refuse {
				if err == nil {
					t.Fatal("a warrant requiring only content was accepted: this element can never " +
						"produce anything for it, and the ADMF has been told an interception is running")
				}
				if !strings.Contains(err.Error(), "X3Only") {
					t.Errorf("the refusal reason is %q; a provisioning function has to be able to "+
						"tell this from a fault", err)
				}

				return
			}
			if err != nil {
				t.Errorf("a warrant this element produces product for was refused: %v", err)
			}
		})
	}
}

// TestATaskWhoseDestinationsCarryNoIRIIsRefused is the third axis of "cannot act on", and the one
// the other two do not cover.
//
// A task can pass both — its identifiers resolve, its delivery type asks for IRI — and still be
// one this element can produce nothing for, because every destination it names is an X3 endpoint.
// The live shape is an ADMF that provisioned a destination for the warrant's CC leg and then named
// it on the IRI task as well.
//
// Acknowledged, such a task is stored, answers `provisioningStatus: complete` with an empty fault
// list, and consumes a sequence number per record while delivering to zero addresses. Every
// account this element gives of itself says the interception is running.
//
// The distinction that must survive: a task naming *no* destination is a gap the provisioning
// function left, and the configured MDF2 fills it. Only a task that named destinations and yielded
// no X2 endpoint is an assertion this element cannot honour.
func TestATaskWhoseDestinationsCarryNoIRIIsRefused(t *testing.T) {
	base := func(dids []string, deliveries []types.DeliveryEndpoint) types.InterceptTask {
		return types.InterceptTask{
			XID:        taskXID,
			Targets:    []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "262019876543210"}},
			Products:   []types.ProductType{types.ProductIRI},
			DIDs:       dids,
			Deliveries: deliveries,
		}
	}

	t.Run("named destinations, none of them X2", func(t *testing.T) {
		err := canApply(base([]string{"did-cc"}, []types.DeliveryEndpoint{
			{Type: types.DeliveryX3, Address: "192.0.2.9:9999"},
		}))
		if err == nil {
			t.Fatal("a task whose only destination is an X3 endpoint was accepted. Every record " +
				"it produces is built, numbered and delivered to nowhere, while this element " +
				"reports the interception as running and faultless")
		}

		if !strings.Contains(err.Error(), "X2") {
			t.Errorf("the refusal reason is %q; a provisioning function has to be able to tell "+
				"this from a fault", err)
		}
	})

	t.Run("named destinations including an X2 one", func(t *testing.T) {
		if err := canApply(base([]string{"did-iri", "did-cc"}, []types.DeliveryEndpoint{
			{Type: types.DeliveryX2, Address: "192.0.2.8:8888"},
			{Type: types.DeliveryX3, Address: "192.0.2.9:9999"},
		})); err != nil {
			t.Errorf("a task naming an X2 endpoint alongside an X3 one was refused: %v", err)
		}
	})

	t.Run("no destinations named at all", func(t *testing.T) {
		// The gap the configured MDF2 fills, which is a different fact and must stay accepted —
		// it is the case of an ADMF that provisions no destinations.
		if err := canApply(base(nil, nil)); err != nil {
			t.Errorf("a task naming no destinations was refused: %v. That is a gap the "+
				"provisioning function left, not an instruction this element cannot honour", err)
		}
	})
}
