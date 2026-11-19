// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/li/iri"
)

// The conditional identity members of TS 33.128 that the AMF holds, and so must
// report.
//
// One assertion per record-field site, deliberately. A single test asking whether
// "some identity is present" passes with all but one of them still missing. A
// record that omits a conditional field is well formed, so nothing downstream can
// notice any of these.

// fullIdentity is a snapshot holding every value the records below can report.
func fullIdentity() amfctx.UeIdentity {
	id := targetIdentity()
	// MCC 262 / MNC 01, routing indicator "0123" (four digits, leading zero),
	// protection scheme 1, home network key 0x1B, scheme output DEADBEEF.
	id.SuciRaw = []byte{0x01, 0x62, 0xf2, 0x10, 0x10, 0x32, 0x01, 0x1B, 0xDE, 0xAD, 0xBE, 0xEF}
	id.TaiList = []models.Tai{{
		PlmnId: models.PlmnId{Mcc: "262", Mnc: "01"},
		Tac:    "002a",
	}}
	id.RatType = models.RATTYPE_NR

	return id
}

func TestEachAMFRecordCarriesTheSUCIItHolds(t *testing.T) {
	id := fullIdentity()

	for _, tc := range []struct {
		record string
		suci   iri.SUCI
	}{
		{"AMFRegistration", amfRegistration(id, iri.RegResult3GPPAccess).SUCI},
		{"AMFDeregistration", amfDeregistration(id, iri.DirNetworkInitiated, iri.AccessThreeGPP).SUCI},
		{"AMFLocationUpdate", amfLocationUpdate(id).SUCI},
		{"AMFUnsuccessfulProcedure", amfUnsuccessfulRegistration(id, 0x0B).SUCI},
		{"AMFStartOfInterceptionWithRegisteredUE", amfStartOfInterception(id, iri.RegResult3GPPAccess).SUCI},
		{"AMFIdentifierAssociation", amfIdentifierAssociation(id).SUCI},
		{"AMFIdentifierDeassociation", amfIdentifierDeassociation(id).SUCI},
		{"AMFUEPolicyTransfer", amfUEPolicyTransfer(id, make([]byte, 16)).SUCI},
	} {
		t.Run(tc.record, func(t *testing.T) {
			if tc.suci.MCC != "262" || tc.suci.MNC != "01" {
				t.Fatalf("%s carries no sUCI (%+v); the AMF holds one", tc.record, tc.suci)
			}
			if tc.suci.RoutingIndicator != 123 {
				t.Errorf("RoutingIndicator = %d, want 123", tc.suci.RoutingIndicator)
			}
			if tc.suci.RoutingIndicatorLength == nil || *tc.suci.RoutingIndicatorLength != 4 {
				t.Errorf("RoutingIndicatorLength not carried; \"0123\" has four meaningful digits "+
					"and the integer 123 renders as three, so the identifier cannot be recovered "+
					"without it (%v)", tc.suci.RoutingIndicatorLength)
			}
		})
	}
}

// The deassociation record's whole purpose is to report that an identifier is no
// longer associated with a subject, so it names as many identifiers for that subject
// as every other record the AMF emits.
func TestDeassociationNamesTheIdentifiersItConcerns(t *testing.T) {
	got := amfIdentifierDeassociation(fullIdentity())
	if got.PEI == nil {
		t.Error("no pEI: the AMF reports one in every other record it emits")
	}
	if got.GPSI == nil {
		t.Error("no gPSI: the AMF reports one in every other record it emits")
	}
	if got.SUPI == nil {
		t.Error("no sUPI")
	}
}

func TestTheThreeRecordsThatCarryATAIList(t *testing.T) {
	id := fullIdentity()

	for _, tc := range []struct {
		record string
		list   iri.TAIList
	}{
		{"AMFRegistration", amfRegistration(id, iri.RegResult3GPPAccess).FiveGSTAIList},
		{"AMFIdentifierAssociation", amfIdentifierAssociation(id).FiveGSTAIList},
		{"AMFStartOfInterceptionWithRegisteredUE", amfStartOfInterception(id, iri.RegResult3GPPAccess).FiveGSTAIList},
	} {
		t.Run(tc.record, func(t *testing.T) {
			if len(tc.list) != 1 {
				t.Fatalf("fiveGSTAIList has %d entries, want 1", len(tc.list))
			}
			tai := tc.list[0]
			if tai.PLMNID.MCC != "262" || tai.PLMNID.MNC != "01" {
				t.Errorf("PLMN = %s/%s, want 262/01", tai.PLMNID.MCC, tai.PLMNID.MNC)
			}
			if len(tai.TAC) != 2 || tai.TAC[0] != 0x00 || tai.TAC[1] != 0x2A {
				t.Errorf("TAC = %x, want 002a", tai.TAC)
			}
		})
	}
}

func TestAMFRegistrationCarriesItsRATType(t *testing.T) {
	if got := amfRegistration(fullIdentity(), iri.RegResult3GPPAccess).RATType; got != iri.RATNR {
		t.Errorf("rATType = %d, want %d (nR)", got, iri.RATNR)
	}
}

// An unmapped satellite RAT would report nothing at all for non-terrestrial
// sessions.
func TestSatelliteRATTypesAreMapped(t *testing.T) {
	for _, tc := range []struct {
		in   models.RatType
		want iri.RATType
	}{
		{models.RATTYPE_NR_LEO, iri.RATNRLEO},
		{models.RATTYPE_NR_MEO, iri.RATNRMEO},
		{models.RATTYPE_NR_GEO, iri.RATNRGEO},
		{models.RATTYPE_NR_OTHER_SAT, iri.RATNROtherSat},
	} {
		if got := ratTypeOf(tc.in); got != tc.want {
			t.Errorf("ratTypeOf(%s) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// The negative. "Conditional on availability" means absent when unheld, not present
// and empty — a UE that registered by 5G-GUTI never presented a SUCI, which is the
// common case.
func TestUnheldConditionalIdentitiesAreAbsentNotEmpty(t *testing.T) {
	bare := targetIdentity() // no SuciRaw, no TaiList, no RatType

	reg := amfRegistration(bare, iri.RegResult3GPPAccess)
	if reg.SUCI.MCC != "" || reg.SUCI.RoutingIndicator != 0 || len(reg.SUCI.SchemeOutput) != 0 {
		t.Errorf("sUCI is present for a UE that never sent one: %+v", reg.SUCI)
	}
	if reg.FiveGSTAIList != nil {
		t.Errorf("fiveGSTAIList is present with no registration area: %+v", reg.FiveGSTAIList)
	}
	if reg.RATType != 0 {
		t.Errorf("rATType = %d, want absent when the AMF holds none", reg.RATType)
	}

	// An unmapped RAT type must report nothing rather than guess nR.
	if got := ratTypeOf(models.RatType("SOMETHING_NEW")); got != 0 {
		t.Errorf("an unknown RAT type mapped to %d; reporting the wrong access is a claim "+
			"about the session that nothing downstream can check", got)
	}
}
