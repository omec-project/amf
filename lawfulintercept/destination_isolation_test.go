// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"

	"github.com/omec-project/util/li/types"
)

// TestTheConfiguredEndpointServesAnUnnamedDestinationOnly is the decision the three
// destination outcomes are one of, asserted where it is made.
//
// A task that names **no** destination is a gap the provisioning function left, and the
// configured endpoint fills it: that is the case of an ADMF that provisions no destinations,
// and it stays served.
//
// A task that names destinations and yields no X2 endpoint is an assertion this element
// cannot honour, and the live shape of it is a warrant naming an X3-only destination —
// the CC half of a warrant whose IRI half this element produces. Substituting the
// configured MDF2 there sends an agency's signalling to an endpoint the warrant never
// named. Both resolve to an empty list, and they are different facts.
func TestTheConfiguredEndpointServesAnUnnamedDestinationOnly(t *testing.T) {
	sub := &subsystem{mdf2: testAgencyAddr}

	named := types.InterceptTask{
		XID:      "11111111-1111-4111-8111-111111111111",
		Products: []types.ProductType{types.ProductIRI, types.ProductCC},
		// Named, and resolved — to a destination that carries content and not signalling.
		DIDs: []string{testOtherDID},
		Deliveries: []types.DeliveryEndpoint{{
			DID: testOtherDID, Type: types.DeliveryX3,
			Address: "10.0.70.9:42069",
		}},
	}
	if got := sub.x2Destinations(named); len(got) != 0 {
		t.Errorf("xIRI for a task naming only an X3 destination goes to %v, want nowhere: the "+
			"element substituted its own configured endpoint for one the warrant named, which on "+
			"an element serving several agencies sends a warrant's signalling to whichever "+
			"address local configuration happens to name", got)
	}

	unnamed := types.InterceptTask{
		XID:      "22222222-2222-4222-8222-222222222222",
		Products: []types.ProductType{types.ProductIRI},
	}
	got := sub.x2Destinations(unnamed)
	if len(got) != 1 || got[0] != testAgencyAddr {
		t.Errorf("xIRI for a task naming no destination goes to %v, want the configured MDF2: an "+
			"ADMF that provisions no destination is the case the fallback exists for, and "+
			"refusing it would be an outage rather than a conformance fix", got)
	}

	// And a task whose named destinations do include an X2 endpoint delivers there, to
	// that endpoint and to no other.
	both := types.InterceptTask{
		XID:      "44444444-4444-4444-8444-444444444444",
		Products: []types.ProductType{types.ProductIRI},
		DIDs:     []string{"55555555-5555-4555-8555-555555555555"},
		Deliveries: []types.DeliveryEndpoint{{
			DID: "55555555-5555-4555-8555-555555555555", Type: types.DeliveryX2,
			Address: "10.0.80.9:42069",
		}},
	}
	got = sub.x2Destinations(both)
	if len(got) != 1 || got[0] != "10.0.80.9:42069" {
		t.Errorf("xIRI went to %v, want only the endpoint the task named", got)
	}
}

// TestAnUnresolvableWarrantDeliversToNoAgency is the cross-agency case that makes the
// unresolvable destination a refusal rather than a report.
//
// The element is serving agency A, whose endpoint is in its configuration as the default.
// Agency B's warrant names a destination this element cannot resolve. If the element
// substitutes its own default, agency B's product — signalling about agency B's subject,
// under agency B's warrant identifier — is delivered to agency A. A fault report does not
// recall it, which is why the remedy is refusal at activation and this test asserts the
// delivery side of the same property: whatever else happens, the configured endpoint
// receives nothing on behalf of a warrant that named a destination.
func TestAnUnresolvableWarrantDeliversToNoAgency(t *testing.T) {
	const agencyA = "10.0.60.122:42069" // in this element's configuration
	sub := &subsystem{mdf2: agencyA}

	// Agency B's warrant, as it would look if x1 had stored it: it named a destination
	// and this element resolved none of it. x1 refuses such a task outright, so this
	// is the belt to that braces — the delivery path must not substitute either.
	agencyBWarrant := types.InterceptTask{
		XID:      "66666666-6666-4666-8666-666666666666",
		Products: []types.ProductType{types.ProductIRI},
		DIDs:     []string{"77777777-7777-4777-8777-777777777777"},
	}

	for _, addr := range sub.x2Destinations(agencyBWarrant) {
		if addr == agencyA {
			t.Fatalf("a second agency's warrant delivers to %s, this element's configured endpoint: "+
				"one agency receives another agency's product, under a warrant it does not hold, "+
				"and no report recalls it", addr)
		}
		t.Errorf("a warrant naming an unresolvable destination delivers to %s", addr)
	}
}
