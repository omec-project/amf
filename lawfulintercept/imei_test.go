// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/util/li/iri"
	"github.com/omec-project/util/li/types"
)

// An IMEI as the AMF holds it: TS 29.571's "imei-" PEI, fifteen digits, the last being the
// check digit (TS 24.008 clause 10.5.1.4: "the IMEI is composed of 15 digits").
const (
	testPEIIMEI = "imei-356938035643809"
	testIMEI14  = "35693803564380"
)

// TestAnIMEIIsReportedWithoutItsCheckDigit: TS 33.128 defines IMEI as NumericString (SIZE(14)),
// the TAC and serial number. All fifteen digits would put every record for a UE identified by
// IMEI out of constraint, and the encoder refuses such a record outright.
func TestAnIMEIIsReportedWithoutItsCheckDigit(t *testing.T) {
	id := targetIdentity()
	id.Pei = testPEIIMEI

	if got := peiChoice(id); got != iri.IMEI(testIMEI14) {
		t.Errorf("PEI = %#v, want the fourteen-digit IMEI %q", got, testIMEI14)
	}
	if _, err := iri.EncodeXIRI(amfRegistration(id, iri.RegResult3GPPAccess)); err != nil {
		t.Errorf("a registration for an IMEI-identified UE is not encodable: %v", err)
	}

	// IMEISV carries no check digit and is reported whole.
	id.Pei = "imeisv-3569380356438091"
	if got := peiChoice(id); got != iri.IMEISV("3569380356438091") {
		t.Errorf("PEI = %#v, want the IMEISV unchanged", got)
	}
}

// TestAFourteenDigitIMEITaskMatches: the X1 peiImei is the fourteen-digit IMEI (TS 103 280's
// IMEI type), while the AMF holds fifteen digits; compared against those, a schema-valid IMEI
// warrant would never match anyone.
func TestAFourteenDigitIMEITaskMatches(t *testing.T) {
	id := amfctx.UeIdentity{Pei: testPEIIMEI}
	task := types.InterceptTask{
		Targets:  []types.TargetIdentifier{{Type: types.TargetPEI, Value: testIMEI14}},
		Products: []types.ProductType{types.ProductIRI},
	}

	if !taskTargets(task, id) {
		t.Errorf("a warrant naming the IMEI %s does not match the UE holding %s", testIMEI14, testPEIIMEI)
	}
}

// TestAnUnknownPLMNLeavesTheOptionalGUTIOut: with neither an assigned GUTI nor a served GUAMI
// the AMF knows no PLMN for the GUTI. The encoder refuses a GUTI with an empty MCC and MNC, so
// a record where the GUTI is optional would be lost over a field it can simply omit.
func TestAnUnknownPLMNLeavesTheOptionalGUTIOut(t *testing.T) {
	amfctx.AMF_Self().ServedGuamiList = nil

	id := amfctx.UeIdentity{Supi: testSUPI, Tmsi: 42}
	if g := fiveGGUTI(id); g != (iri.FiveGGUTI{}) {
		t.Errorf("GUTI with no PLMN = %+v, want the zero GUTI, which an optional member omits", g)
	}
	if _, err := iri.EncodeXIRI(amfDeregistration(id, iri.DirUEInitiated, iri.AccessThreeGPP)); err != nil {
		t.Errorf("a deregistration was refused over a GUTI it could omit: %v", err)
	}
}
