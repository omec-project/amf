// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/li/iri"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

const testTargetSUPI = "262019876543210"

// errDelivery stands in for an MDF that cannot be reached.
var errDelivery = errors.New("MDF unreachable")

func targetUE() *amfctx.AmfUe {
	return &amfctx.AmfUe{
		Supi: "imsi-" + testTargetSUPI,
		Pei:  testPEI,
		Gpsi: testGPSI,
		// A registered UE holds the GUTI its AMF assigned: PLMN 262-01, AMF identifier
		// 010203, 5G-TMSI 42. Without one the record's mandatory GUTI would be empty, which
		// the encoder refuses — as a conformant mediation function would.
		Guti: testGUTI,
		Tmsi: 42,
	}
}

// activateIRI installs a subsystem with one IRI warrant for supi, delivering
// everything to snd.
func activateIRI(t *testing.T, snd sender, supi string) {
	t.Helper()
	st := store.New()
	if !st.Activate(types.InterceptTask{
		XID:      testSubstitutionXID,
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: supi}},
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	}) {
		t.Fatal("activate")
	}
	active.Store(&subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	})
	t.Cleanup(func() { active.Store(nil) })
}

func TestReportServiceAccept(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, testTargetSUPI)

	// The 5G-TMSI the record names is the one in the GUTI the UE holds, not ue.Tmsi.
	ue := targetUE()
	ue.Tmsi = 1
	ReportServiceAccept(ue, nasMessage.ServiceTypeMobileTerminatedServices)

	records := decodeRecords(t, snd)
	if len(records) != 1 {
		t.Fatalf("delivered %d records, want 1", len(records))
	}
	rec := records[0]
	if rec.event != eventUEServiceAccept {
		t.Fatalf("delivered XIRIEvent [%d], want aMFUEServiceAccept [%d]", rec.event, eventUEServiceAccept)
	}
	// TS 33.128 table 6.2.2.2.13-1: serviceType per TS 24.501 clause 9.11.3.50, and the
	// 5G-TMSI "if known".
	if got := rec.member(t, 3).Bytes; len(got) != 1 || got[0] != nasMessage.ServiceTypeMobileTerminatedServices {
		t.Errorf("serviceType = % x, want %02x", got, nasMessage.ServiceTypeMobileTerminatedServices)
	}
	if got := integer(t, rec.member(t, 4)); got != 42 {
		t.Errorf("fiveGTMSI = %d, want 42 from the UE's GUTI", got)
	}
	if got := supiOf(t, rec.member(t, 1)); got != testTargetSUPI {
		t.Errorf("SUPI = %q", got)
	}
	// The message-type octet, per TS 24.501 clause 9.7 — not the whole PDU.
	id := only(t, "serviceMessageIdentity [2]", rec.member(t, 2))
	if id.Tag != 2 || len(id.Bytes) != 1 {
		t.Fatalf("serviceMessageIdentity holds [%d] % x, want a one-octet serviceAccept [2] arm", id.Tag, id.Bytes)
	}
}

func sampleHandover() Handover {
	return Handover{
		UE:                targetUE(),
		AMFUENGAPID:       7,
		RANUENGAPID:       9,
		TargetAMFUENGAPID: 11,
		TargetRANUENGAPID: 13,
		HandoverType:      1,
		TargetToSource:    []byte{0xDE, 0xAD, 0x00},
		SourceToTarget:    []byte{0x00, 0xBE, 0xEF},
		PDUSessionID:      5,
		CauseGroup:        CauseGroupRadioNetwork,
		CauseValue:        17,
		HasCause:          true,
		HasPDUSessionID:   true,
		HasSourceToTarget: true,
	}
}

func TestReportHandoverRecords(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, testTargetSUPI)

	h := sampleHandover()
	ReportHandoverRequest(h)
	ReportHandoverCommand(h)

	records := decodeRecords(t, snd)
	if len(records) != 2 {
		t.Fatalf("delivered %d records, want 2", len(records))
	}

	req := records[0]
	if req.event != eventRANHandoverRequest {
		t.Fatalf("first record is XIRIEvent [%d], want aMFRANHandoverRequest [%d]", req.event, eventRANHandoverRequest)
	}
	// The target node's association, which the acknowledge arrived over.
	if amf, ran := integer(t, req.member(t, 2)), integer(t, req.member(t, 3)); amf != 11 || ran != 13 {
		t.Errorf("NGAP ids = %d/%d, want the target association's 11/13", amf, ran)
	}
	if got := req.member(t, 11).Bytes; !bytes.Equal(got, h.SourceToTarget) {
		t.Errorf("sourceToTargetContainer = % x — the container carried from HANDOVER REQUIRED", got)
	}
	if got := req.member(t, 9).Bytes; !bytes.Equal(got, h.TargetToSource) {
		t.Errorf("targetToSourceContainer = % x", got)
	}
	// handoverCause [5] EXPLICIT around the group's alternative: radioNetwork [1].
	if cause := only(t, "handoverCause [5]", req.member(t, 5)); cause.Tag != 1 || integer(t, cause) != 17 {
		t.Errorf("handoverCause holds [%d] %d, want radioNetwork [1] 17", cause.Tag, integer(t, cause))
	}
	if id := only(t, "pDUSessionResourceInformation [6]", req.member(t, 6)); id.Tag != 1 || integer(t, id) != 5 {
		t.Errorf("pDUSessionResourceInformation holds [%d] %d, want pDUSessionID [1] 5", id.Tag, integer(t, id))
	}

	cmd := records[1]
	if cmd.event != eventRANHandoverCommand {
		t.Fatalf("second record is XIRIEvent [%d], want aMFRANHandoverCommand [%d]", cmd.event, eventRANHandoverCommand)
	}
	// The source node's association, which the command travels over.
	if amf, ran := integer(t, cmd.member(t, 2)), integer(t, cmd.member(t, 3)); amf != 7 || ran != 9 {
		t.Errorf("command NGAP ids = %d/%d, want the source association's 7/9", amf, ran)
	}
	if got := cmd.member(t, 5).Bytes; !bytes.Equal(got, h.TargetToSource) {
		t.Errorf("command targetToSourceContainer = % x", got)
	}
	if supiOf(t, cmd.member(t, 1)) != testTargetSUPI {
		t.Error("the two handover records must name the same subscriber")
	}
}

// TestHandoverRequestNeedsEveryMandatoryMember: eight members are mandatory, and
// three of them are carried from HANDOVER REQUIRED. If a carried one is missing —
// a handover whose REQUIRED was never seen, or whose state was already cleared —
// the record cannot be completed: none is emitted, and the warrant is told the record
// was not produced rather than left to find a handover missing. The command record has
// no carried members and is unaffected.
func TestHandoverRequestNeedsEveryMandatoryMember(t *testing.T) {
	missing := map[string]func(*Handover){
		"no cause":            func(h *Handover) { h.HasCause = false },
		"no PDU session":      func(h *Handover) { h.HasPDUSessionID = false },
		"no source container": func(h *Handover) { h.HasSourceToTarget, h.SourceToTarget = false, nil },
	}
	for name, break_ := range missing {
		t.Run(name, func(t *testing.T) {
			admf := newADMFStub(t)
			snd := &captureSender{}
			activateIRIReporting(t, snd, admf)
			h := sampleHandover()
			break_(&h)
			ReportHandoverRequest(h)
			if len(snd.pdus) != 0 {
				t.Errorf("an incomplete handover produced %d record(s)", len(snd.pdus))
			}
			if got := admf.awaitReport(t, "recordNotProduced"); !strings.Contains(got, "recordNotProduced") {
				t.Errorf("an incomplete handover was not reported as recordNotProduced:\n%s", got)
			}
		})
	}
}

// TestAnEmptyTargetToSourceContainerIsReportedAsTheRANSentIt: the AMF copies the transparent
// containers, and TS 38.413 sets them no minimum length, so a RAN may answer the HANDOVER
// REQUEST with an empty target-to-source container — gnbsim rel-2.3.0 does. That is the RAN's
// value, not a missing one: both records are delivered carrying it, and nothing is reported as
// lost.
func TestAnEmptyTargetToSourceContainerIsReportedAsTheRANSentIt(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, testTargetSUPI)

	h := sampleHandover()
	h.TargetToSource = []byte{}
	ReportHandoverRequest(h)
	ReportHandoverCommand(h)

	records := decodeRecords(t, snd)
	if len(records) != 2 {
		t.Fatalf("delivered %d records for a handover whose RAN sent an empty container, want 2", len(records))
	}
	if got := records[0].member(t, 9).Bytes; len(got) != 0 {
		t.Errorf("request targetToSourceContainer = % x, want the empty container the RAN sent", got)
	}
	if got := records[1].member(t, 5).Bytes; len(got) != 0 {
		t.Errorf("command targetToSourceContainer = % x, want the empty container the RAN sent", got)
	}
}

// TestHandoverCauseGroupsAreDistinguished: the group is half the meaning. A value
// alone cannot tell "radio network: handover desirable" from "misc: hardware
// failure".
//
// Each group is exercised at its own last permitted value, which is both the sharpest boundary
// to encode and the only way the case is expressible at all: the groups have four different
// upper bounds, and a single value shared across all five — 3, say — is outside
// `CauseTransport`, whose enumeration has two values, and the bound refuses it.
func TestHandoverCauseGroupsAreDistinguished(t *testing.T) {
	groups := []struct {
		group HandoverCauseGroup
		value int64
		arm   int // the HandoverCause alternative: radioNetwork [1] … misc [5]
	}{
		{CauseGroupRadioNetwork, 52, 1},
		{CauseGroupTransport, 2, 2},
		{CauseGroupNAS, 4, 3},
		{CauseGroupProtocol, 7, 4},
		{CauseGroupMisc, 6, 5},
	}
	for _, g := range groups {
		snd := &captureSender{}
		activateIRI(t, snd, testTargetSUPI)
		h := sampleHandover()
		h.CauseGroup, h.CauseValue = g.group, g.value
		ReportHandoverRequest(h)

		records := decodeRecords(t, snd)
		if len(records) != 1 {
			t.Fatalf("group %v: delivered %d records", g.group, len(records))
		}
		cause := only(t, "handoverCause [5]", records[0].member(t, 5))
		if cause.Tag != g.arm || integer(t, cause) != g.value {
			t.Errorf("group %v is carried as [%d] %d, want [%d] %d",
				g.group, cause.Tag, integer(t, cause), g.arm, g.value)
		}
	}
}

// TestNewRecordsSilentForUntaskedSubscriber is the undetectability assertion for
// every LI hook in the AMF: a subscriber under no warrant must
// produce nothing on any of these paths. A record that appeared only for tasked
// subscribers would be a perfect detector, which is the one outcome the rules
// forbid absolutely.
func TestNewRecordsSilentForUntaskedSubscriber(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, "999999999999999") // someone else

	ReportServiceAccept(targetUE(), nasMessage.ServiceTypeData)
	ue := targetUE()
	NoteUEPolicyCommand(ue, uePolicyCommandOf(1, 16))
	ReportUEPolicyComplete(ue, []byte{1, uePolicyComplete})
	ReportHandoverRequest(sampleHandover())
	ReportHandoverCommand(sampleHandover())

	if len(snd.pdus) != 0 {
		t.Errorf("an untasked subscriber produced %d record(s)", len(snd.pdus))
	}
}

// TestNewRecordsSurviveMissingContext: these hooks sit on paths that run for
// every UE, so they must tolerate a nil context and an absent subsystem without
// panicking — a panic here is a service outage, not an LI defect.
func TestNewRecordsSurviveMissingContext(t *testing.T) {
	t.Run("no subsystem", func(t *testing.T) {
		active.Store(nil)
		ReportServiceAccept(targetUE(), nasMessage.ServiceTypeData)
		NoteUEPolicyCommand(targetUE(), uePolicyCommandOf(1, 16))
		ReportUEPolicyComplete(targetUE(), []byte{1, uePolicyComplete})
		ReportHandoverRequest(sampleHandover())
		ReportHandoverCommand(sampleHandover())
	})
	t.Run("nil UE", func(t *testing.T) {
		activateIRI(t, &captureSender{}, testTargetSUPI)
		ReportServiceAccept(nil, nasMessage.ServiceTypeData)
		NoteUEPolicyCommand(nil, uePolicyCommandOf(1, 16))
		ReportUEPolicyComplete(nil, []byte{1, uePolicyComplete})
		h := sampleHandover()
		h.UE = nil
		ReportHandoverRequest(h)
		ReportHandoverCommand(h)
	})
	t.Run("delivery fails", func(t *testing.T) {
		activateIRI(t, senderFunc(func(*x2x3.PDU) error { return errDelivery }), testTargetSUPI)
		ReportServiceAccept(targetUE(), nasMessage.ServiceTypeData)
		ReportHandoverCommand(sampleHandover())
	})
}

// TestPeriodicRegistrationIsReportedOnceAsPeriodic: a periodic registration update is a
// registration procedure the AMF performs, so TS 33.128 wants a record for it, and
// suppressing it would be a silent omission — an agency cannot tell an event that
// was not reported from a subject who did nothing. It is reported by the
// registration-complete tap rather than by the mobility tap, because the
// registration accept carries a 5G-GUTI whenever the UE has one and the UE
// therefore answers with Registration Complete.
func TestPeriodicRegistrationIsReportedOnceAsPeriodic(t *testing.T) {
	for _, tc := range []struct {
		name    string
		regType uint8
		want    iri.AMFRegistrationType
	}{
		{"periodic", nasMessage.RegistrationType5GSPeriodicRegistrationUpdating, iri.RegTypePeriodic},
		{"mobility", nasMessage.RegistrationType5GSMobilityRegistrationUpdating, iri.RegTypeMobility},
		{"initial", nasMessage.RegistrationType5GSInitialRegistration, iri.RegTypeInitial},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snd := &captureSender{}
			activateIRI(t, snd, testTargetSUPI)

			ue := targetUE()
			ue.RegistrationType5GS = tc.regType
			ReportRegistration(ue, models.ACCESSTYPE__3_GPP_ACCESS)

			if len(snd.pdus) != 1 {
				t.Fatalf("delivered %d records for one registration, want exactly 1", len(snd.pdus))
			}
			if got := registrationType(ue.IdentitySnapshot()); got != tc.want {
				t.Errorf("registration type = %v, want %v — an agency reads this field to "+
					"tell a keepalive from a movement from a new attachment", got, tc.want)
			}
		})
	}
}
