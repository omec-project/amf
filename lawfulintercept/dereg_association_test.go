// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"

	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/li/iri"
)

// TestADeregistrationRecordNamesTheScopeTheElementActedOn is the mapping half: the AMF
// acts on the access a UE-originating deregistration *asked for* — AccessTypeBoth
// releases the SM contexts of both accesses — so the record names that, not the access
// the NAS message happened to arrive on.
//
// TS 33.128 table 6.2.2.2.3-1 makes accessType mandatory with cardinality 1, and the
// type carries three values: AccessType ::= ENUMERATED { threeGPPAccess(1),
// nonThreeGPPAccess(2), threeGPPandNonThreeGPPAccess(3) }. So "both" is a value of the
// single field, not a reason to emit two records — one xIRI, naming what was done. The
// clause's own trigger text agrees: the record is generated when a UE "has deregistered
// from the 5GS over at least one access type".
//
// A record contradicting what the same function then does is not the declarable case of
// a field the element cannot populate. It is a populated field asserting something
// false, and nothing downstream distinguishes the two.
func TestADeregistrationRecordNamesTheScopeTheElementActedOn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		nasValue uint8
		want     iri.AccessType
	}{
		{"both accesses", nasMessage.AccessTypeBoth, iri.AccessBoth},
		{"3GPP only", nasMessage.AccessType3GPP, iri.AccessThreeGPP},
		{"non-3GPP only", nasMessage.AccessTypeNon3GPP, iri.AccessNonThreeGPP},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeregistrationScope(tc.nasValue); got != tc.want {
				t.Errorf("a deregistration requesting %s is reported as %v, want %v",
					tc.name, got, tc.want)
			}
		})
	}

	// An unrecognised value is not guessed at: the caller falls back to the access it
	// is handling rather than this inventing a scope.
	if got := DeregistrationScope(0x7f); got != 0 {
		t.Errorf("an unrecognised access type produced scope %v; the caller can no longer tell "+
			"that this did not answer", got)
	}

	// And the single-access mapper still answers for the paths that act on one access.
	if got := AccessScope(models.ACCESSTYPE_NON_3_GPP_ACCESS); got != iri.AccessNonThreeGPP {
		t.Errorf("AccessScope(non-3GPP) = %v", got)
	}
}
