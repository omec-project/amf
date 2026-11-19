// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"strings"
	"sync"
	"testing"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"github.com/omec-project/util/li/iri"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x1"
	"github.com/omec-project/util/li/x2x3"
)

// Identities and addresses shared across this package's tests.
const (
	testNEID        = "amf-1"
	testADMFID      = "admf-1"
	testAgencyAddr  = "10.0.60.122:42069"
	testAgency2Addr = "10.0.60.99:42069"
	testX1Listen    = "127.0.0.1:0"
	testOtherDID    = "33333333-3333-4333-8333-333333333333"
	testSUPI        = "imsi-" + testTargetSUPI
	testPEI         = "imeisv-3534250000000151"
	testGPSI        = "msisdn-4915123456789"
	// testGUTI is the 5G-GUTI a registered test UE holds: PLMN 262-01, AMF identifier 010203,
	// 5G-TMSI 42. A record's mandatory GUTI cannot be empty.
	testGUTI        = "26201" + "010203" + "0000002a"
	testX2Only      = "X2Only"
	testKeepaliveP2 = "30s"
)

// captureSender records the xIRI PDUs delivered, standing in for the X2 client
// so tests can assert per-warrant delivery isolation.
type captureSender struct{ pdus []*x2x3.PDU }

func (c *captureSender) Send(p *x2x3.PDU) error {
	c.pdus = append(c.pdus, p)
	return nil
}

// targetIdentity is a fully-identified UE identity snapshot used across the
// mapping tests.
func targetIdentity() amfctx.UeIdentity {
	return amfctx.UeIdentity{
		Supi: testSUPI,
		Pei:  testPEI,
		Gpsi: testGPSI,
		// The GUTI its AMF assigned; see targetUE.
		Guti: testGUTI,
		Tmsi: 42,
	}
}

func TestTargetsOf(t *testing.T) {
	id := amfctx.UeIdentity{
		Supi: testSUPI,
		Pei:  testPEI,
		Gpsi: testGPSI,
	}
	got := targetsOf(id)
	// The IMEISV the UE presented, and the IMEI it carries (its first fourteen digits), so an
	// IMEI warrant matches this device as well as an IMEISV one.
	want := map[types.TargetIdentifier]bool{
		{Type: types.TargetSUPI, Value: "262019876543210"}: true,
		{Type: types.TargetPEI, Value: "3534250000000151"}: true,
		{Type: types.TargetPEI, Value: "35342500000001"}:   true,
		{Type: types.TargetGPSI, Value: "4915123456789"}:   true,
	}
	if len(got) != len(want) {
		t.Fatalf("targetsOf returned %d ids, want %d: %+v", len(got), len(want), got)
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("unexpected identifier %+v", id)
		}
	}

	// An unmappable/absent identifier is not emitted.
	if ids := targetsOf(amfctx.UeIdentity{Supi: "suci-0-262-01-..."}); len(ids) != 0 {
		t.Errorf("unmappable SUPI produced identifiers: %+v", ids)
	}
}

func TestAMFRegistrationMapping(t *testing.T) {
	id := amfctx.UeIdentity{
		Supi: testSUPI,
		Pei:  "imei-353425000000015",
		Gpsi: testGPSI,
	}
	reg := amfRegistration(id, iri.RegResult3GPPAccess)
	if reg.RegistrationResult != iri.RegResult3GPPAccess {
		t.Errorf("registrationResult = %d", reg.RegistrationResult)
	}
	if supi, ok := reg.SUPI.(iri.IMSI); !ok || supi != iri.IMSI("262019876543210") {
		t.Errorf("SUPI = %#v", reg.SUPI)
	}
	// Fourteen digits: TS 33.128's IMEI has no check digit. See imeiWithoutCheckDigit.
	if pei, ok := reg.PEI.(iri.IMEI); !ok || pei != iri.IMEI("35342500000001") {
		t.Errorf("PEI = %#v", reg.PEI)
	}
	if gpsi, ok := reg.GPSI.(iri.MSISDN); !ok || gpsi != iri.MSISDN("4915123456789") {
		t.Errorf("GPSI = %#v", reg.GPSI)
	}
}

func TestFiveGGUTIDecode(t *testing.T) {
	// AmfId "010203" = RegionID(0x01)=1, SetID(bits 15..6)=8, Pointer(0x03)=3.
	amfctx.AMF_Self().ServedGuamiList = []models.Guami{
		{PlmnId: models.PlmnIdNid{Mcc: "262", Mnc: "01"}, AmfId: "010203"},
	}
	t.Cleanup(func() { amfctx.AMF_Self().ServedGuamiList = nil })

	g := fiveGGUTI(amfctx.UeIdentity{Tmsi: 42})
	if g.MCC != "262" || g.MNC != "01" {
		t.Errorf("PLMN = %s/%s, want 262/01", g.MCC, g.MNC)
	}
	if g.AMFRegionID != 1 || g.AMFSetID != 8 || g.AMFPointer != 3 {
		t.Errorf("AmfId decode = region %d set %d pointer %d, want 1/8/3", g.AMFRegionID, g.AMFSetID, g.AMFPointer)
	}
	if g.FiveGTMSI != 42 {
		t.Errorf("5G-TMSI = %d, want 42", g.FiveGTMSI)
	}
}

func TestParseXID(t *testing.T) {
	x := parseXID("50b93d1e-1b53-4d63-aacb-e4d99811bc0b")
	if x[0] != 0x50 || x[15] != 0x0b {
		t.Errorf("parseXID = % x", x)
	}
	if got := parseXID("not-a-uuid"); got != ([16]byte{}) {
		t.Errorf("bad XID = % x, want zero", got)
	}
}

func TestRegistrationType(t *testing.T) {
	cases := map[uint8]iri.AMFRegistrationType{
		nasMessage.RegistrationType5GSInitialRegistration:          iri.RegTypeInitial,
		nasMessage.RegistrationType5GSMobilityRegistrationUpdating: iri.RegTypeMobility,
		nasMessage.RegistrationType5GSPeriodicRegistrationUpdating: iri.RegTypePeriodic,
		nasMessage.RegistrationType5GSEmergencyRegistration:        iri.RegTypeEmergency,
		nasMessage.RegistrationType5GSReserved:                     iri.RegTypeInitial, // unknown → initial
	}
	for nas, want := range cases {
		if got := registrationType(amfctx.UeIdentity{RegistrationType5GS: nas}); got != want {
			t.Errorf("registrationType(%d) = %d, want %d", nas, got, want)
		}
	}
}

// TestRegistrationEventDispatch checks that a mobility registration update maps
// to an AMFLocationUpdate and every other registration type to AMFRegistration.
func TestRegistrationEventDispatch(t *testing.T) {
	id := targetIdentity()

	id.RegistrationType5GS = nasMessage.RegistrationType5GSMobilityRegistrationUpdating
	if _, ok := registrationEvent(id, iri.RegResult3GPPAccess).(iri.AMFLocationUpdate); !ok {
		t.Errorf("mobility update → %T, want AMFLocationUpdate", registrationEvent(id, iri.RegResult3GPPAccess))
	}

	for _, rt := range []uint8{
		nasMessage.RegistrationType5GSInitialRegistration,
		nasMessage.RegistrationType5GSPeriodicRegistrationUpdating,
	} {
		id.RegistrationType5GS = rt
		reg, ok := registrationEvent(id, iri.RegResult3GPPAccess).(iri.AMFRegistration)
		if !ok {
			t.Errorf("registration type %d → %T, want AMFRegistration", rt, registrationEvent(id, iri.RegResult3GPPAccess))
			continue
		}
		if rt == nasMessage.RegistrationType5GSPeriodicRegistrationUpdating && reg.RegistrationType != iri.RegTypePeriodic {
			t.Errorf("periodic registration → regType %d, want %d", reg.RegistrationType, iri.RegTypePeriodic)
		}
	}
}

func TestDeregistrationMapping(t *testing.T) {
	id := targetIdentity()

	net := amfDeregistration(id, iri.DirNetworkInitiated, iri.AccessThreeGPP)
	if net.DeregistrationDirection != iri.DirNetworkInitiated || net.AccessType != iri.AccessThreeGPP {
		t.Errorf("network dereg = dir %d access %d", net.DeregistrationDirection, net.AccessType)
	}
	if supi, ok := net.SUPI.(iri.IMSI); !ok || supi != "262019876543210" {
		t.Errorf("dereg SUPI = %#v", net.SUPI)
	}

	ue2 := amfDeregistration(id, iri.DirUEInitiated, iri.AccessNonThreeGPP)
	if ue2.DeregistrationDirection != iri.DirUEInitiated || ue2.AccessType != iri.AccessNonThreeGPP {
		t.Errorf("ue dereg = dir %d access %d", ue2.DeregistrationDirection, ue2.AccessType)
	}
}

func TestUnsuccessfulRegistrationMapping(t *testing.T) {
	rec := amfUnsuccessfulRegistration(targetIdentity(), nasMessage.Cause5GMM5GSServicesNotAllowed)
	if rec.FailedProcedureType != iri.FailedRegistration {
		t.Errorf("failedProcedureType = %d, want FailedRegistration", rec.FailedProcedureType)
	}
	cause, ok := rec.FailureCause.(iri.FiveGMMCause)
	if !ok || cause != iri.FiveGMMCause(nasMessage.Cause5GMM5GSServicesNotAllowed) {
		t.Errorf("failureCause = %#v, want 5GMM %d", rec.FailureCause, nasMessage.Cause5GMM5GSServicesNotAllowed)
	}
}

func TestAccessTypeMapping(t *testing.T) {
	if accessType(models.ACCESSTYPE__3_GPP_ACCESS) != iri.AccessThreeGPP {
		t.Error("3GPP access mismapped")
	}
	if accessType(models.ACCESSTYPE_NON_3_GPP_ACCESS) != iri.AccessNonThreeGPP {
		t.Error("non-3GPP access mismapped")
	}
}

func TestTaskTargets(t *testing.T) {
	id := targetIdentity()
	hit := types.InterceptTask{Targets: []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "262019876543210"}}}
	miss := types.InterceptTask{Targets: []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "000000000000000"}}}
	if !taskTargets(hit, id) {
		t.Error("matching SUPI task not recognised")
	}
	if taskTargets(miss, id) {
		t.Error("non-matching task falsely recognised")
	}

	// All of a task's criteria must match (TS 103 221-1 table 6.2.1.2-1). This predicate decides
	// the activation scan, a record's re-validation and whether a UE is covered.
	both := types.InterceptTask{Targets: []types.TargetIdentifier{
		{Type: types.TargetSUPI, Value: "262019876543210"},
		{Type: types.TargetPEI, Value: "3534250000000151"},
	}}
	if !taskTargets(both, id) {
		t.Error("a task naming this UE's SUPI and PEI was not recognised")
	}
	otherPEI := types.InterceptTask{Targets: []types.TargetIdentifier{
		{Type: types.TargetSUPI, Value: "262019876543210"},
		{Type: types.TargetPEI, Value: "9999999999999999"},
	}}
	if taskTargets(otherPEI, id) {
		t.Error("a task naming a PEI this UE does not present matched on its SUPI alone")
	}
}

func TestRegistered(t *testing.T) {
	dereg := &amfctx.AmfUe{State: map[models.AccessType]*fsm.State{
		models.ACCESSTYPE__3_GPP_ACCESS:    fsm.NewState(amfctx.Deregistered),
		models.ACCESSTYPE_NON_3_GPP_ACCESS: fsm.NewState(amfctx.Deregistered),
	}}
	if got := registeredAccesses(dereg); len(got) != 0 {
		t.Errorf("deregistered UE reported as registered over %v", got)
	}

	reg := &amfctx.AmfUe{State: map[models.AccessType]*fsm.State{
		models.ACCESSTYPE__3_GPP_ACCESS:    fsm.NewState(amfctx.Registered),
		models.ACCESSTYPE_NON_3_GPP_ACCESS: fsm.NewState(amfctx.Deregistered),
	}}
	if got := registeredAccesses(reg); len(got) != 1 || got[0] != iri.RegResult3GPPAccess {
		t.Errorf("UE registered on 3GPP access reported as registered over %v", got)
	}

	// A UE with no State map must not panic and is not registered.
	if got := registeredAccesses(&amfctx.AmfUe{}); len(got) != 0 {
		t.Errorf("UE with nil State reported as registered over %v", got)
	}
}

func TestIdentifierAssociationMapping(t *testing.T) {
	amfctx.AMF_Self().ServedGuamiList = []models.Guami{
		{PlmnId: models.PlmnIdNid{Mcc: "262", Mnc: "01"}, AmfId: "010203"},
	}
	t.Cleanup(func() { amfctx.AMF_Self().ServedGuamiList = nil })

	id := targetIdentity()
	id.Tmsi = 42

	assoc := amfIdentifierAssociation(id)
	if supi, ok := assoc.SUPI.(iri.IMSI); !ok || supi != "262019876543210" {
		t.Errorf("association SUPI = %#v", assoc.SUPI)
	}
	if assoc.GUTI.FiveGTMSI != 42 || assoc.GUTI.MCC != "262" {
		t.Errorf("association GUTI = %+v", assoc.GUTI)
	}

	deassoc := amfIdentifierDeassociation(id)
	if supi, ok := deassoc.SUPI.(iri.IMSI); !ok || supi != "262019876543210" {
		t.Errorf("deassociation SUPI = %#v", deassoc.SUPI)
	}
	if deassoc.GUTI.FiveGTMSI != 42 {
		t.Errorf("deassociation GUTI = %+v", deassoc.GUTI)
	}
}

// TestDeliveryIsolation checks multi-agency isolation: two agencies tasking the
// same target each receive exactly their own xIRI tagged with their own XID
// (no cross-delivery), and a CC-only warrant never leaks into IRI (X2) delivery.
func TestDeliveryIsolation(t *testing.T) {
	const (
		xidA  = "aaaaaaaa-0000-0000-0000-000000000001"
		xidB  = "bbbbbbbb-0000-0000-0000-000000000002"
		xidCC = "cccccccc-0000-0000-0000-000000000003"
	)
	target := types.TargetIdentifier{Type: types.TargetSUPI, Value: "262019876543210"}
	st := store.New()
	st.Activate(types.InterceptTask{XID: xidA, Targets: []types.TargetIdentifier{target}, Products: []types.ProductType{types.ProductIRI}, State: types.TaskActive})
	st.Activate(types.InterceptTask{XID: xidB, Targets: []types.TargetIdentifier{target}, Products: []types.ProductType{types.ProductIRI}, State: types.TaskActive})
	st.Activate(types.InterceptTask{XID: xidCC, Targets: []types.TargetIdentifier{target}, Products: []types.ProductType{types.ProductCC}, State: types.TaskActive})

	capture := &captureSender{}
	// No task here names a destination, so all three fall back to the configured MDF2 —
	// the path of an ADMF that provisions no destinations, which this test therefore also
	// pins.
	active.Store(&subsystem{
		ids:   x2x3.NewIdentity(testNEID, amfInterceptionPoint),
		store: st, senderFor: func(string) sender { return capture },
		mdf2: testAgencyAddr,
	})
	t.Cleanup(func() { active.Store(nil) })

	// Exercised through the exported entry point, so the snapshot is taken the
	// way a live NAS path takes it.
	ReportRegistration(&amfctx.AmfUe{
		Supi: testSUPI,
		Pei:  testPEI,
		Gpsi: testGPSI,
		Guti: testGUTI,
		Tmsi: 42,
	}, models.ACCESSTYPE__3_GPP_ACCESS)

	if len(capture.pdus) != 2 {
		t.Fatalf("delivered %d xIRI PDUs, want 2 (the two IRI agencies; CC-only excluded)", len(capture.pdus))
	}
	count := map[[16]byte]int{}
	for _, p := range capture.pdus {
		count[p.XID]++
	}
	if count[parseXID(xidA)] != 1 || count[parseXID(xidB)] != 1 {
		t.Errorf("each IRI agency must receive its own xIRI exactly once; XID counts = %v", count)
	}
	if count[parseXID(xidCC)] != 0 {
		t.Error("CC-only warrant leaked into IRI (X2) delivery")
	}
}

// TestEncodeAllEvents verifies every AMF xIRI a reporter can produce encodes
// through the real TS 33.128 encoder without error — i.e. mandatory members are
// present and CHOICE arms are registered. This is the correctness check that a
// pure-mapping test cannot give.
func TestEncodeAllEvents(t *testing.T) {
	id := targetIdentity()
	events := map[string]any{
		"registration":            amfRegistration(id, iri.RegResult3GPPAccess),
		"locationUpdate":          amfLocationUpdate(id),
		"deregistration":          amfDeregistration(id, iri.DirNetworkInitiated, iri.AccessThreeGPP),
		"unsuccessful":            amfUnsuccessfulRegistration(id, nasMessage.Cause5GMM5GSServicesNotAllowed),
		"startOfInterception":     amfStartOfInterception(id, iri.RegResult3GPPAccess),
		"identifierAssociation":   amfIdentifierAssociation(id),
		"identifierDeassociation": amfIdentifierDeassociation(id),
	}
	for name, ev := range events {
		if _, err := iri.EncodeXIRI(ev); err != nil {
			t.Errorf("encode %s: %v", name, err)
		}
	}
}

// TestFiveGGUTIPrefersTheUEsOwnGUTI: an AMF may serve several GUAMIs, and the UE's
// GUTI was not necessarily cut from the first of them. Reporting the served list's
// head would tell the agency a 5G-GUTI the UE is not known by; the UE's own GUTI string
// is the identifier the network actually assigned it.
func TestFiveGGUTIPrefersTheUEsOwnGUTI(t *testing.T) {
	// Two served GUAMIs; the UE's GUTI comes from the second.
	amfctx.AMF_Self().ServedGuamiList = []models.Guami{
		{PlmnId: models.PlmnIdNid{Mcc: "262", Mnc: "01"}, AmfId: "010203"},
		{PlmnId: models.PlmnIdNid{Mcc: "310", Mnc: "260"}, AmfId: "0a0b0c"},
	}
	t.Cleanup(func() { amfctx.AMF_Self().ServedGuamiList = nil })

	// mcc(310) + mnc(260) + amfId(0a0b0c) + 5G-TMSI(8 hex).
	g := fiveGGUTI(amfctx.UeIdentity{Guti: "3102600a0b0c0000002a", Tmsi: 42})
	if g.MCC != "310" || g.MNC != "260" {
		t.Errorf("PLMN = %s/%s, want 310/260 from the UE's own GUTI", g.MCC, g.MNC)
	}
	// 0x0a0b0c = RegionID 0x0a=10, SetID (bits 15..6) = 0x0b0c>>6 = 44, Pointer 0x0c&0x3f = 12.
	if g.AMFRegionID != 10 || g.AMFSetID != 44 || g.AMFPointer != 12 {
		t.Errorf("AmfId decode = region %d set %d pointer %d, want 10/44/12", g.AMFRegionID, g.AMFSetID, g.AMFPointer)
	}

	// A two-digit MNC is the other valid width.
	if g := fiveGGUTI(amfctx.UeIdentity{Guti: "262010102030000002a", Tmsi: 42}); g.MCC != "262" || g.MNC != "01" {
		t.Errorf("two-digit-MNC GUTI = %s/%s, want 262/01", g.MCC, g.MNC)
	}

	// No GUTI yet: fall back to the served list rather than emitting nothing.
	if g := fiveGGUTI(amfctx.UeIdentity{Tmsi: 42}); g.MCC != "262" || g.MNC != "01" {
		t.Errorf("fallback PLMN = %s/%s, want the served GUAMI 262/01", g.MCC, g.MNC)
	}

	// No GUTI and no served GUAMI: leave the PLMN unset rather than emit an empty
	// NumericString, which the schema forbids.
	amfctx.AMF_Self().ServedGuamiList = nil
	if g := fiveGGUTI(amfctx.UeIdentity{Tmsi: 42}); g.MCC != "" || g.MNC != "" {
		t.Errorf("PLMN = %s/%s with nothing to derive it from, want empty", g.MCC, g.MNC)
	}
}

// TestMobilityUpdateIsReportedOnce: a mobility registration update is reported as
// an AMFLocationUpdate from the mobility handler. It also reaches
// HandleRegistrationComplete — BuildRegistrationAccept carries the 5G-GUTI IE
// whenever the UE has one, and TS 24.501 clause 5.5.1.3.4 then requires the UE to
// send Registration Complete — so reporting from both would give the agency two
// location records for one movement. This pins the dispatch that makes the
// duplicate visible if the guard in HandleRegistrationComplete is ever removed.
func TestMobilityUpdateIsReportedOnce(t *testing.T) {
	id := amfctx.UeIdentity{
		Supi:                testSUPI,
		RegistrationType5GS: nasMessage.RegistrationType5GSMobilityRegistrationUpdating,
	}
	if _, ok := registrationEvent(id, iri.RegResult3GPPAccess).(iri.AMFLocationUpdate); !ok {
		t.Fatalf("mobility update produced %T, want iri.AMFLocationUpdate", registrationEvent(id, iri.RegResult3GPPAccess))
	}

	// Which is why HandleRegistrationComplete must not also report it: the two call
	// sites would emit the same record twice for one procedure. An initial
	// registration is the case that call site exists for.
	id.RegistrationType5GS = nasMessage.RegistrationType5GSInitialRegistration
	if _, ok := registrationEvent(id, iri.RegResult3GPPAccess).(iri.AMFRegistration); !ok {
		t.Errorf("initial registration produced %T, want iri.AMFRegistration", registrationEvent(id, iri.RegResult3GPPAccess))
	}
}

// addressCapture records what was delivered and, crucially, where. The subject of these
// tests is the destination, and a capture that forgets the address cannot see a misdelivery:
// with one configured endpoint, delivering to the task's destination and delivering to
// configuration look identical.
type addressCapture struct {
	mu   sync.Mutex
	sent map[string][][16]byte
}

func newAddressCapture() *addressCapture {
	return &addressCapture{sent: map[string][][16]byte{}}
}

func (c *addressCapture) senderFor(addr string) sender {
	return senderFunc(func(p *x2x3.PDU) error {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.sent[addr] = append(c.sent[addr], p.XID)

		return nil
	})
}

type senderFunc func(*x2x3.PDU) error

func (f senderFunc) Send(p *x2x3.PDU) error { return f(p) }

func x2To(addr string) []types.DeliveryEndpoint {
	return []types.DeliveryEndpoint{{Type: types.DeliveryX2, Address: addr}}
}

// TestXIRIGoesToTheDestinationsTheTaskNamed: two warrants, two agencies, and neither agency
// sees the other's subscriber.
//
// Delivering both tasks to the AMF's own configured MDF2, though both resolved their
// destinations, fails this: an ADMF that provisioned two endpoints would get everything at
// one of them, which is a disclosure to an agency with no warrant for it.
func TestXIRIGoesToTheDestinationsTheTaskNamed(t *testing.T) {
	const (
		xidA    = "aaaaaaaa-0000-0000-0000-000000000001"
		xidB    = "bbbbbbbb-0000-0000-0000-000000000002"
		agencyA = "10.0.60.122:42069"
		agencyB = "10.0.60.123:42070"
	)
	target := types.TargetIdentifier{Type: types.TargetSUPI, Value: "262019876543210"}
	st := store.New()
	st.Activate(types.InterceptTask{
		XID: xidA, Targets: []types.TargetIdentifier{target},
		Products: []types.ProductType{types.ProductIRI}, Deliveries: x2To(agencyA),
	})
	st.Activate(types.InterceptTask{
		XID: xidB, Targets: []types.TargetIdentifier{target},
		Products: []types.ProductType{types.ProductIRI}, Deliveries: x2To(agencyB),
	})

	capture := newAddressCapture()
	active.Store(&subsystem{
		ids:   x2x3.NewIdentity(testNEID, amfInterceptionPoint),
		store: st, senderFor: capture.senderFor,
		// Configured, and deliberately neither agency's address: if the task's destinations
		// were ignored, this is where both records would arrive, and the assertion below would say so.
		mdf2: testAgency2Addr,
	})
	t.Cleanup(func() { active.Store(nil) })

	ReportRegistration(&amfctx.AmfUe{Supi: testSUPI, Guti: testGUTI, Tmsi: 42}, models.ACCESSTYPE__3_GPP_ACCESS)

	for _, c := range []struct{ addr, xid string }{{agencyA, xidA}, {agencyB, xidB}} {
		got := capture.sent[c.addr]
		if len(got) != 1 {
			t.Errorf("%s received %d records, want 1", c.addr, len(got))

			continue
		}
		if got[0] != parseXID(types.XID(c.xid)) {
			t.Errorf("%s received a record for a warrant it was not provisioned for", c.addr)
		}
	}
	if n := len(capture.sent[testAgency2Addr]); n != 0 {
		t.Errorf("the configured endpoint received %d records, want 0: both tasks named a destination", n)
	}
}

// The configured endpoint stays as the fallback. An ADMF that provisions no destinations
// depends on it, and taking this path away would turn that deployment into an outage.
//
// The fallback keys on the task's DIDs, not on its resolved deliveries (see x2Destinations),
// so two shapes reach it: a task with no destinations at all, and one that names no DID and
// carries only a content delivery. Both resolve no X2 endpoint and name no destination, so
// both fall back.
func TestATaskNamingNoDestinationFallsBackToConfiguration(t *testing.T) {
	for _, c := range []struct {
		name       string
		deliveries []types.DeliveryEndpoint
	}{
		{name: "the task named no destination at all"},
		{
			name:       "the task named only a destination that carries content",
			deliveries: []types.DeliveryEndpoint{{Type: types.DeliveryX3, Address: testAgencyAddr}},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			target := types.TargetIdentifier{Type: types.TargetSUPI, Value: "262019876543210"}
			st := store.New()
			st.Activate(types.InterceptTask{
				XID: testSubstitutionXID, Targets: []types.TargetIdentifier{target},
				Products: []types.ProductType{types.ProductIRI}, Deliveries: c.deliveries,
			})

			capture := newAddressCapture()
			active.Store(&subsystem{
				ids:   x2x3.NewIdentity(testNEID, amfInterceptionPoint),
				store: st, senderFor: capture.senderFor,
				mdf2: testAgency2Addr,
			})
			t.Cleanup(func() { active.Store(nil) })

			ReportRegistration(&amfctx.AmfUe{Supi: testSUPI, Guti: testGUTI, Tmsi: 42}, models.ACCESSTYPE__3_GPP_ACCESS)

			if n := len(capture.sent[testAgency2Addr]); n != 1 {
				t.Errorf("the configured endpoint received %d records, want 1", n)
			}
			// And nothing went to the content endpoint: signalling delivered there would
			// be a disclosure to an endpoint the ADMF designated for something else.
			if n := len(capture.sent[testAgencyAddr]); n != 0 {
				t.Errorf("the X3 endpoint received %d xIRI records, want 0", n)
			}
		})
	}
}

// TS 33.128 clause 6.2.2.2.1 scopes an AMF task's records to what its
// IdentifierAssociationExtensions asked for: the identifier-association pair "shall not be
// generated" absent the extension, so it goes to no task that did not ask for it.
func TestRecordScopeDecidesWhichRecordsATaskReceives(t *testing.T) {
	for _, c := range []struct {
		name                         string
		scope                        types.RecordScope
		wantGeneral, wantAssociation int
	}{
		{"no extension", types.RecordScopeStandard, 1, 0},
		{"IdentifierAssociation only", types.RecordScopeIdentifierAssociation, 0, 1},
		{"All", types.RecordScopeAll, 1, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			target := types.TargetIdentifier{Type: types.TargetSUPI, Value: "262019876543210"}
			st := store.New()
			st.Activate(types.InterceptTask{
				XID: testSubstitutionXID, Targets: []types.TargetIdentifier{target},
				Products: []types.ProductType{types.ProductIRI}, RecordScope: c.scope,
			})

			capture := &captureSender{}
			active.Store(&subsystem{
				ids:   x2x3.NewIdentity(testNEID, amfInterceptionPoint),
				store: st, senderFor: func(string) sender { return capture },
				mdf2: testAgency2Addr,
			})
			t.Cleanup(func() { active.Store(nil) })

			ue := &amfctx.AmfUe{Supi: testSUPI, Guti: testGUTI, Tmsi: 42}
			ReportRegistration(ue, models.ACCESSTYPE__3_GPP_ACCESS) // a general record
			if got := len(capture.pdus); got != c.wantGeneral {
				t.Errorf("registration delivered %d records, want %d", got, c.wantGeneral)
			}

			before := len(capture.pdus)
			ReportIdentifierAssociation(ue)
			if got := len(capture.pdus) - before; got != c.wantAssociation {
				t.Errorf("identifier association delivered %d records, want %d", got, c.wantAssociation)
			}
		})
	}
}

// TestDeliveryFaultIsReportedOnBothEdges covers this element's contribution to its own
// status answer, and all three assertions are the point rather than one of them.
//
// A probe stuck *off* leaves an element that has been failing to deliver for hours
// answering that nothing is wrong — invisible, and the reason an ADMF can ask at all. A
// probe stuck *on* makes every healthy element report itself faulty, which is noticed
// immediately and discredits the whole field. So two of the three assertions below are
// about the probe staying quiet.
func TestDeliveryFaultIsReportedOnBothEdges(t *testing.T) {
	unreachable := 0
	sub := &subsystem{unreachable: func() (int, int) { return unreachable, 2 }}

	if fault := sub.deliveryFault(); fault != nil {
		t.Errorf("with both destinations reachable the element reports itself faulty: %q",
			fault.ErrorDescription)
	}

	unreachable = 1
	fault := sub.deliveryFault()
	if fault == nil {
		t.Fatal("with a destination unreachable the element reports no fault; an ADMF cannot " +
			"tell it apart from one delivering every record")
	}
	if !strings.Contains(fault.ErrorDescription, x1.NEIssueMDFUnreachable) {
		t.Errorf("the fault does not name the condition: %q", fault.ErrorDescription)
	}
	if !strings.Contains(fault.ErrorDescription, "1 of 2") {
		t.Errorf("the fault does not say how much is wrong: %q", fault.ErrorDescription)
	}

	// Nothing clears it. Delivery starts working and the next answer says so, which is the
	// property no design that remembers faults can offer.
	unreachable = 0
	if fault := sub.deliveryFault(); fault != nil {
		t.Errorf("the fault outlived the condition, with nothing having cleared it: %q",
			fault.ErrorDescription)
	}
}

// TestDeliveryFaultNamesNoDestination keeps the NE-level answer at NE level. This element
// may deliver two agencies' warrants to two MDF2s; TS 103 221-1 keeps an element's own
// status separate from per-destination and per-task faults, and an answer naming the failing
// address would put interception detail in a message that is not scoped to a warrant.
func TestDeliveryFaultNamesNoDestination(t *testing.T) {
	sub := &subsystem{unreachable: func() (int, int) { return 1, 2 }}

	fault := sub.deliveryFault()
	if fault == nil {
		t.Fatal("no fault reported for an unreachable destination")
	}
	for _, identity := range []string{"10.0.60.122", "42069", "262019876543210"} {
		if strings.Contains(fault.ErrorDescription, identity) {
			t.Errorf("the element's own status names %q; it must say how much is wrong, never whose",
				identity)
		}
	}
}

// TestDeliveryFaultWithNoAccountingIsSilent: an element that cannot say is not an element
// that is broken. The probe runs on the X1 request goroutine, where reporting a fault
// nobody observed — or panicking — are both worse than answering that nothing is known.
func TestDeliveryFaultWithNoAccountingIsSilent(t *testing.T) {
	if fault := (&subsystem{}).deliveryFault(); fault != nil {
		t.Errorf("an element with no delivery accounting reported a fault: %q", fault.ErrorDescription)
	}
}

// TestDestinationsInUseFollowsTheTasking is what keeps the delivery probe from sticking on.
//
// A delivery client outlives the warrant that created it — nothing removes one — so a
// destination whose last delivery failed and whose warrant was then deactivated could never
// be delivered to again, and nothing would ever clear it. The element would report itself
// faulty for the life of the process, including while holding no tasking at all, which is
// precisely the failure that gets a status answer ignored.
func TestDestinationsInUseFollowsTheTasking(t *testing.T) {
	st := store.New()
	sub := &subsystem{store: st, mdf2: testAgency2Addr}

	if got := sub.destinationsInUse(); len(got) != 0 {
		t.Errorf("an element holding no tasking delivers to %v, want nothing", got)
	}

	// A warrant naming its agency's own endpoint.
	st.Activate(types.InterceptTask{
		XID:      "aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa",
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "208930100007488"}},
		Products: []types.ProductType{types.ProductIRI},
		Deliveries: []types.DeliveryEndpoint{
			{Type: types.DeliveryX2, Address: testAgencyAddr},
		},
	})
	if got := sub.destinationsInUse(); len(got) != 1 || got[0] != testAgencyAddr {
		t.Errorf("destinationsInUse() = %v, want the endpoint the warrant named", got)
	}

	// A warrant naming no destination is delivered to the configured endpoint, so that
	// is where product goes and what the probe must ask about.
	st.Activate(types.InterceptTask{
		XID:      "bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb",
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "208930100007489"}},
		Products: []types.ProductType{types.ProductIRI},
	})
	got := sub.destinationsInUse()
	if len(got) != 2 {
		t.Fatalf("destinationsInUse() = %v, want both warrants' endpoints", got)
	}

	// Both warrants end. Whatever their delivery clients last established, this element no
	// longer delivers anywhere.
	st.Deactivate("aaaaaaaa-1111-4111-8111-aaaaaaaaaaaa")
	st.Deactivate("bbbbbbbb-2222-4222-8222-bbbbbbbbbbbb")
	if got := sub.destinationsInUse(); len(got) != 0 {
		t.Errorf("after every warrant was withdrawn the element still delivers to %v; a client "+
			"left failing there would report a fault nothing could clear", got)
	}
}

// awaitScans makes a test own the UE-pool scans it starts.
//
// reportStartOfInterception launches a goroutine — deliberately, so that acknowledging a
// warrant does not wait on a walk of every UE this element holds — and tracks it on
// subsystem.scans. A test that returns without waiting leaves it running into the next test,
// where it reads `scanWalked` while that test writes it. `go test -race` reports the write and
// the read from two different tests, and names the *second* test, which has nothing wrong with
// it: the failure moves with scheduling.
//
// Registered as cleanup rather than called inline, so it changes no assertion: the tests that
// need the scan's *output* already poll for it, and this only holds the test open until the
// goroutine it started has finished.
func awaitScans(t *testing.T, s *subsystem) {
	t.Helper()
	t.Cleanup(func() { s.scans.Wait() })
}
