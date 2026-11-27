// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"github.com/omec-project/util/li/iri"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

// ueRegisteredOver is the target UE with the given accesses in the Registered state and the
// others Deregistered.
func ueRegisteredOver(registered ...models.AccessType) *amfctx.AmfUe {
	ue := targetUE()
	ue.State = map[models.AccessType]*fsm.State{
		models.ACCESSTYPE__3_GPP_ACCESS:    fsm.NewState(amfctx.Deregistered),
		models.ACCESSTYPE_NON_3_GPP_ACCESS: fsm.NewState(amfctx.Deregistered),
	}
	for _, access := range registered {
		ue.State[access] = fsm.NewState(amfctx.Registered)
	}

	return ue
}

// An AMFRegistration's registrationResult is mandatory, and names the accesses the UE is
// registered over once this registration completes, as the REGISTRATION ACCEPT's 5GS
// registration result does. A constant 3GPP value would report a non-3GPP or dual
// registration as something it is not.
func TestARegistrationRecordNamesTheAccessesTheUEIsRegisteredOver(t *testing.T) {
	for _, c := range []struct {
		name       string
		completeOn models.AccessType
		registered []models.AccessType
		want       iri.AMFRegistrationResult
	}{
		{"over 3GPP", models.ACCESSTYPE__3_GPP_ACCESS, nil, iri.RegResult3GPPAccess},
		{"over non-3GPP", models.ACCESSTYPE_NON_3_GPP_ACCESS, nil, iri.RegResultNon3GPPAccess},
		{
			name:       "over non-3GPP while registered over 3GPP",
			completeOn: models.ACCESSTYPE_NON_3_GPP_ACCESS,
			registered: []models.AccessType{models.ACCESSTYPE__3_GPP_ACCESS},
			want:       iri.RegResult3GPPAndNon3GPP,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			snd := &captureSender{}
			activateIRI(t, snd, testTargetSUPI)

			ReportRegistration(ueRegisteredOver(c.registered...), c.completeOn)

			records := decodeRecords(t, snd)
			if len(records) != 1 {
				t.Fatalf("delivered %d records, want 1", len(records))
			}
			// AMFRegistration: registrationResult is member [2].
			if got := integer(t, records[0].member(t, 2)); got != int64(c.want) {
				t.Errorf("registrationResult = %d, want %d", got, c.want)
			}
		})
	}
}

// TS 33.128 clause 6.2.2.2.5: "If the UE is registered over both 3GPP NG-RAN and non-3GPP
// access, the IRI-POI present in the AMF shall generate an xIRI containing an
// AMFStartOfInterceptionWithRegisteredUE record for each access type." So the activation
// scan makes one record per access the UE is registered over, each naming that access.
func TestAStartOfInterceptionIsMadeForEachAccessTheUEIsRegisteredOver(t *testing.T) {
	for _, c := range []struct {
		name       string
		registered []models.AccessType
		want       []iri.AMFRegistrationResult
	}{
		{"over 3GPP", []models.AccessType{models.ACCESSTYPE__3_GPP_ACCESS}, []iri.AMFRegistrationResult{iri.RegResult3GPPAccess}},
		{"over non-3GPP", []models.AccessType{models.ACCESSTYPE_NON_3_GPP_ACCESS}, []iri.AMFRegistrationResult{iri.RegResultNon3GPPAccess}},
		{
			name:       "over both",
			registered: []models.AccessType{models.ACCESSTYPE__3_GPP_ACCESS, models.ACCESSTYPE_NON_3_GPP_ACCESS},
			want:       []iri.AMFRegistrationResult{iri.RegResult3GPPAccess, iri.RegResultNon3GPPAccess},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			task := types.InterceptTask{
				XID:      testSubstitutionXID,
				Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: testTargetSUPI}},
				Products: []types.ProductType{types.ProductIRI},
				State:    types.TaskActive,
			}
			st := store.New()
			if !st.Activate(task) {
				t.Fatal("activate")
			}
			snd := &captureSender{}
			s := &subsystem{
				store: st, senderFor: func(string) sender { return snd },
				mdf2: testAgencyAddr,
				ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
			}
			amfctx.AMF_Self().UePool.Store(testTargetSUPI, ueRegisteredOver(c.registered...))
			t.Cleanup(func() { amfctx.AMF_Self().UePool.Delete(testTargetSUPI) })

			s.reportStartOfInterception(task, nil)
			s.scans.Wait()

			records := decodeRecords(t, snd)
			if len(records) != len(c.want) {
				t.Fatalf("delivered %d records, want %d", len(records), len(c.want))
			}
			for i, want := range c.want {
				// AMFStartOfInterceptionWithRegisteredUE: registrationResult is member [1].
				if got := integer(t, records[i].member(t, 1)); got != int64(want) {
					t.Errorf("record %d: registrationResult = %d, want %d", i, got, want)
				}
			}
		})
	}
}

// TS 33.128 clause 6.2.2.2.6: "Unsuccessful registration shall be reported only if the target
// UE has been successfully authenticated." A reject sent to a UE holding no valid NAS
// security context — one that failed authentication, or was refused before it — is a reject
// for an identity nobody has proven, and draws no record.
func TestARegistrationRejectIsReportedOnlyForAnAuthenticatedUE(t *testing.T) {
	for _, c := range []struct {
		name          string
		authenticated bool
		macFailed     bool
		want          int
	}{
		{"authenticated", true, false, 1},
		{"not authenticated", false, false, 0},
		// A request that failed its integrity check proves nothing, whatever context the
		// AMF held for the UE before it.
		{"holding a context the request failed", true, true, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			snd := &captureSender{}
			activateIRI(t, snd, testTargetSUPI)

			ue := targetUE()
			ue.NgKsi = models.NgKsi{Ksi: nasMessage.NasKeySetIdentifierNoKeyIsAvailable}
			if c.authenticated {
				ue.SecurityContextAvailable = true
				ue.NgKsi = models.NgKsi{Ksi: 1}
			}
			ue.MacFailed = c.macFailed

			ReportRegistrationReject(ue, nasMessage.Cause5GMMProtocolErrorUnspecified)

			if got := len(snd.pdus); got != c.want {
				t.Errorf("delivered %d records, want %d", got, c.want)
			}
		})
	}
}

// TestARejectCarriesOnlyTheGUTIItUsed: table 6.2.2.2.6-1's gUTI is the "5G-GUTI used in the
// procedure, if available". A UE registering by SUCI used none, so the GUTI the AMF allocated
// for it, which a reject never carries to the UE, is left out; a UE registering by 5G-GUTI used
// the one it presented.
func TestARejectCarriesOnlyTheGUTIItUsed(t *testing.T) {
	const gutiMember = 8
	// SUCI: IMSI format, PLMN 262/01, routing indicator 0123, scheme 1, key 0x1B, output.
	suci := []byte{0x01, 0x62, 0xf2, 0x10, 0x10, 0x32, 0x01, 0x1B, 0xDE, 0xAD, 0xBE, 0xEF}
	// 5G-GUTI: PLMN 262/01, AMF identifier 010203, 5G-TMSI 0x0badf00d.
	guti := []byte{0xf2, 0x62, 0xf2, 0x10, 0x01, 0x02, 0x03, 0x0b, 0xad, 0xf0, 0x0d}
	for _, c := range []struct {
		name     string
		identity []byte
		wantTMSI int64
	}{
		{"by SUCI", suci, -1},
		{"by 5G-GUTI", guti, 0x0badf00d},
	} {
		t.Run(c.name, func(t *testing.T) {
			snd := &captureSender{}
			activateIRI(t, snd, testTargetSUPI)

			ue := targetUE() // holds testGUTI, which this registration did not use
			ue.SecurityContextAvailable = true
			ue.NgKsi = models.NgKsi{Ksi: 1}
			ue.RegistrationRequest = nasMessage.NewRegistrationRequest(0)
			ue.RegistrationRequest.MobileIdentity5GS.SetLen(uint16(len(c.identity)))
			ue.RegistrationRequest.MobileIdentity5GS.Buffer = c.identity

			ReportRegistrationReject(ue, nasMessage.Cause5GMMProtocolErrorUnspecified)

			records := decodeRecords(t, snd)
			if len(records) != 1 {
				t.Fatalf("delivered %d records, want 1", len(records))
			}
			g, present := records[0].members[gutiMember]
			if c.wantTMSI < 0 {
				if present {
					t.Errorf("the reject carries a 5G-GUTI the registration did not use: % x", g.Bytes)
				}
				return
			}
			if !present {
				t.Fatal("the reject carries no 5G-GUTI, though the registration used one")
			}
			// FiveGGUTI's last member is fiveGTMSI [6].
			members := elements(t, "gUTI", g.Bytes)
			if got := integer(t, members[len(members)-1]); got != c.wantTMSI {
				t.Errorf("5G-TMSI = %#x, want the presented %#x", got, c.wantTMSI)
			}
		})
	}
}
