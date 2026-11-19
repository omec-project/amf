// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"

	"github.com/omec-project/util/li/store"
)

// TestTheX1AnswerCarriesThisElementsOwnDeliveryFault asserts the wiring, not the mechanism.
//
// li/x1 renders deliveryFault when it is given a reachability answer, and that is tested
// there. What this element has to get right is *supplying* it: without the option wired into
// newX1Server, the X1 endpoint answers activeAndWorking for an endpoint this same element is
// reporting unreachable over ReportDestinationIssue — a remedy that exists in the library and
// not in the element.
//
// So it goes through newX1Server, the constructor Init uses, for the reason the bulk-operation
// test gives: a second copy of the wiring is exactly where a supplied answer stops being
// supplied without anything failing to compile.
func TestTheX1AnswerCarriesThisElementsOwnDeliveryFault(t *testing.T) {
	const (
		admf = testADMFID
		did  = "11111111-1111-1111-1111-111111111111"
		addr = "10.0.60.122:42069"
	)

	st := store.New()
	down := false
	sub := &subsystem{
		store:         st,
		unreachableAt: func(a string) bool { return down && a == addr },
	}
	cfg := Config{
		NEID: testNEID, AdmfID: admf,
		Destinations: []Destination{{DID: did, DeliveryType: testX2Only, Address: addr}},
	}
	srv := newX1Server(st, cfg, sub)

	ask := func(t *testing.T) bool {
		t.Helper()
		resp, err := srv.Process(bulkRequest("GetAllDestinationDetailsRequest"),
			admfPeerCert(t))
		if err != nil {
			t.Fatalf("Process: %v", err)
		}
		if resp.Messages[0].ErrorInformation != nil {
			t.Fatalf("interrogation refused: %s", resp.Messages[0].ErrorInformation.ErrorDescription)
		}
		if n := len(resp.Messages[0].Destinations); n != 1 {
			t.Fatalf("the element reported %d destinations, want the 1 it is configured with", n)
		}

		return resp.Messages[0].Destinations[0].Unreachable
	}

	if ask(t) {
		t.Error("a reachable destination was reported unreachable")
	}

	down = true

	if !ask(t) {
		t.Error("the element answers that a destination is working while its own delivery " +
			"layer says it cannot be reached; an ADMF checking a pushed fault report by " +
			"interrogation is told the report was wrong")
	}
}
