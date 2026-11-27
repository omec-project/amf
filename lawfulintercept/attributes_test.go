// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"time"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/li/iri"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

// attrsOf indexes a PDU's conditional attributes by type, keeping every occurrence:
// the two target identifier attributes are the ones clause 5.3.1 permits more than
// one of, and collapsing them would hide exactly what these tests are about.
func attrsOf(pdu *x2x3.PDU) map[uint16][]string {
	out := make(map[uint16][]string, len(pdu.Attributes))
	for _, a := range pdu.Attributes {
		out[a.Type] = append(out[a.Type], string(a.Value))
	}

	return out
}

func seqOf(t *testing.T, pdu *x2x3.PDU) uint32 {
	t.Helper()
	for _, a := range pdu.Attributes {
		if a.Type == x2x3.AttrSequenceNumber {
			return binary.BigEndian.Uint32(a.Value)
		}
	}
	t.Fatal("PDU carries no sequence number")

	return 0
}

// activateIRIWithTargets installs a warrant naming exactly the identifiers given,
// with as many delivery destinations as addrs names, delivering everything to snd.
func activateIRIWithTargets(t *testing.T, snd sender, targets []types.TargetIdentifier, addrs ...string) {
	t.Helper()
	task := types.InterceptTask{
		XID:      testSubstitutionXID,
		Targets:  targets,
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	}
	for _, addr := range addrs {
		task.Deliveries = append(task.Deliveries, types.DeliveryEndpoint{Type: types.DeliveryX2, Address: addr})
	}
	st := store.New()
	if !st.Activate(task) {
		t.Fatal("activate")
	}
	active.Store(&subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	})
	t.Cleanup(func() { active.Store(nil) })
}

// TestXIRICarriesTheSixRequiredAttributes is TS 33.128 table 5.3.2-2 on the
// signalling path. Without them the payload is conformant and the header it
// travels in is not.
func TestXIRICarriesTheSixRequiredAttributes(t *testing.T) {
	snd := &captureSender{}
	activateIRIWithTargets(t, snd, []types.TargetIdentifier{{Type: types.TargetSUPI, Value: testTargetSUPI}})

	ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)
	if len(snd.pdus) != 1 {
		t.Fatalf("delivered %d records, want 1", len(snd.pdus))
	}
	attrs := attrsOf(snd.pdus[0])

	// The NFID is the identifier this element asserts on X1, so the mediation function
	// and the ADMF name the same element.
	if got := attrs[x2x3.AttrNFID]; len(got) != 1 || got[0] != testNEID {
		t.Errorf("NFID = %q, want the configured network element identifier", got)
	}
	if got := attrs[x2x3.AttrIPID]; len(got) != 1 || got[0] != amfInterceptionPoint {
		t.Errorf("IPID = %q, want %q", got, amfInterceptionPoint)
	}
	if got := attrs[x2x3.AttrTimestamp]; len(got) != 1 || len(got[0]) != 8 {
		t.Errorf("timestamp = %q, want one 8-octet timespec", got)
	}
	if got := attrs[x2x3.AttrSequenceNumber]; len(got) != 1 {
		t.Errorf("sequence number occurrences = %d, want 1", len(got))
	}

	// The matched identity is the one the task named and the UE presents; the other
	// identities are the rest of *this subject's*, one attribute each.
	if got := attrs[x2x3.AttrMatchedTargetIdentifier]; len(got) != 1 || got[0] != "<supiimsi>"+testTargetSUPI+"</supiimsi>" {
		t.Errorf("matched target identifier = %q, want the SUPI the task named", got)
	}
	// The PEI twice: the IMEISV the UE presented, and the IMEI it carries, which is the
	// device's identity an IMEI warrant names.
	wantOther := map[string]bool{
		"<peiImeisv>3534250000000151</peiImeisv>": true,
		derivedIMEIFragment:                       true,
		"<gpsiMsisdn>4915123456789</gpsiMsisdn>":  true,
	}
	other := attrs[x2x3.AttrOtherTargetIdentifier]
	if len(other) != len(wantOther) {
		t.Errorf("other target identifiers = %q, want %d of them", other, len(wantOther))
	}
	for _, got := range other {
		if !wantOther[got] {
			t.Errorf("unexpected other target identifier %q", got)
		}
	}
}

// TestEveryMatchedIdentityIsReported: where a task names two identities the subject
// presents, both matched. Reporting one and calling it the match is a claim the
// element cannot support, and clause 5.3.18 permits multiple occurrences so that it
// need not make one.
func TestEveryMatchedIdentityIsReported(t *testing.T) {
	snd := &captureSender{}
	activateIRIWithTargets(t, snd, []types.TargetIdentifier{
		{Type: types.TargetSUPI, Value: testTargetSUPI},
		{Type: types.TargetGPSI, Value: "4915123456789"},
	})

	ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)
	if len(snd.pdus) != 1 {
		t.Fatalf("delivered %d records, want 1", len(snd.pdus))
	}
	attrs := attrsOf(snd.pdus[0])

	if got := len(attrs[x2x3.AttrMatchedTargetIdentifier]); got != 2 {
		t.Errorf("matched identities = %d (%q), want both the task named", got, attrs[x2x3.AttrMatchedTargetIdentifier])
	}
	if got := attrs[x2x3.AttrOtherTargetIdentifier]; len(got) != 2 ||
		got[0] != "<peiImeisv>3534250000000151</peiImeisv>" || got[1] != derivedIMEIFragment {
		t.Errorf("other identities = %q, want only the PEI, as the IMEISV and the IMEI it carries", got)
	}
}

// TestOnlyTheSubjectsIdentitiesAreReported guards the reading of "all other target
// identities present at the NF" that would put every subscriber the AMF holds into
// every header. That is not a cost problem, it is a disclosure: an agency holding a
// warrant for one subject would receive the identities of others.
func TestOnlyTheSubjectsIdentitiesAreReported(t *testing.T) {
	snd := &captureSender{}
	activateIRIWithTargets(t, snd, []types.TargetIdentifier{{Type: types.TargetSUPI, Value: testTargetSUPI}})

	// A second, untasked subscriber known to this AMF at the same time.
	other := &amfctx.AmfUe{Supi: "imsi-262010000000999", Pei: "imeisv-3534250000000999", Gpsi: "msisdn-4915100000999"}
	ReportRegistration(other, models.ACCESSTYPE__3_GPP_ACCESS)
	ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)

	if len(snd.pdus) != 1 {
		t.Fatalf("delivered %d records, want 1 — only the tasked subject", len(snd.pdus))
	}
	attrs := attrsOf(snd.pdus[0])
	reported := append(attrs[x2x3.AttrMatchedTargetIdentifier], attrs[x2x3.AttrOtherTargetIdentifier]...)
	for _, frag := range reported {
		for _, leaked := range []string{"262010000000999", "3534250000000999", "4915100000999"} {
			if strings.Contains(frag, leaked) {
				t.Errorf("header carries %q, which belongs to a subscriber this warrant does not name", frag)
			}
		}
	}
}

// TestSequenceNumbersAreOnePerRecordNotPerDestination: the number belongs to the
// (XID, Correlation ID) context, so a record delivered to two of a task's
// destinations carries one number at both. A counter per connection would give the
// same record two different numbers and a mediation function no way to tell that they
// are one record.
func TestSequenceNumbersAreOnePerRecordNotPerDestination(t *testing.T) {
	snd := &captureSender{}
	activateIRIWithTargets(t, snd,
		[]types.TargetIdentifier{{Type: types.TargetSUPI, Value: testTargetSUPI}},
		testAgencyAddr, "10.0.60.123:42069")

	ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)
	if len(snd.pdus) != 2 {
		t.Fatalf("delivered %d copies, want one per destination", len(snd.pdus))
	}
	if a, b := seqOf(t, snd.pdus[0]), seqOf(t, snd.pdus[1]); a != b {
		t.Errorf("the same record was numbered %d and %d", a, b)
	}

	// The next record in the same context is the next number, once — not once per
	// destination.
	ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)
	if got := seqOf(t, snd.pdus[2]); got != 1 {
		t.Errorf("second record numbered %d, want 1: the numbering counted destinations", got)
	}
}

// droppingSender delivers every PDU but the second, standing in for the bounded
// delivery queue discarding product while the MDF is slower than the offered rate.
type droppingSender struct {
	seen   int
	kept   []*x2x3.PDU
	dropAt int
}

func (d *droppingSender) Send(pdu *x2x3.PDU) error {
	d.seen++
	if d.seen == d.dropAt {
		return nil // dropped, exactly as a full queue drops it
	}
	d.kept = append(d.kept, pdu)

	return nil
}

// TestDroppedProductLeavesAGap: the number is taken where the record is built, not
// where it is written, so product lost to a full delivery queue shows up at the
// mediation function as a missing number. Renumbering to close the gap would make
// loss invisible — and loss that cannot be seen cannot be distinguished from a
// subject who did nothing.
func TestDroppedProductLeavesAGap(t *testing.T) {
	snd := &droppingSender{dropAt: 2}
	activateIRIWithTargets(t, snd, []types.TargetIdentifier{{Type: types.TargetSUPI, Value: testTargetSUPI}})

	for range 3 {
		ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)
	}

	if len(snd.kept) != 2 {
		t.Fatalf("delivered %d records, want 2 of 3", len(snd.kept))
	}
	if got := seqOf(t, snd.kept[0]); got != 0 {
		t.Errorf("first delivered record numbered %d, want 0", got)
	}
	if got := seqOf(t, snd.kept[1]); got != 2 {
		t.Errorf("record after the dropped one numbered %d, want 2 — the gap was closed over", got)
	}
}

// TestInitRefusesWithoutAnElementIdentifier: an element that cannot say
// which network function produced a record does not deliver one. The refusal is a
// returned error rather than a fatal start-up, because the AMF must carry on serving
// traffic — a network function that crash-loops over its LI configuration announces to
// every operator with log access that it is LI-provisioned.
func TestInitRefusesWithoutAnElementIdentifier(t *testing.T) {
	if err := Init(Config{X1Listen: testX1Listen}); !errors.Is(err, errNoElementIdentifier) {
		t.Errorf("Init without a network element identifier returned %v, want errNoElementIdentifier", err)
	}
	if active.Load() != nil {
		t.Error("interception is running after a refused initialisation")
	}
}

// TestDeactivationForgetsTheNumbering is the leak guard for the sequence numbering.
// The state is per (XID, Correlation ID) context, so a warrant that outlives many
// sessions would otherwise leave one entry behind for each of them, for the life of
// the process. It is dropped through the X1 deactivation hook, so this drives it the
// way an ADMF does rather than by calling the hook directly.
func TestDeactivationForgetsTheNumbering(t *testing.T) {
	const xid, admf = "11111111-1111-4111-8111-111111111111", testADMFID
	snd := &captureSender{}
	st := store.New()
	st.Activate(types.InterceptTask{
		XID:      xid,
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: testTargetSUPI}},
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	})
	sub := &subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		neID: testNEID, ids: x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	}
	srv := newX1Server(st, Config{NEID: testNEID, AdmfID: admf}, sub)
	active.Store(sub)
	t.Cleanup(func() { active.Store(nil) })

	ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)
	if len(snd.pdus) != 1 || sub.ids.Contexts() != 1 {
		t.Fatalf("delivered %d records over %d contexts, want 1 and 1", len(snd.pdus), sub.ids.Contexts())
	}

	resp, err := srv.Process(bulkRequest("DeactivateAllTasksRequest"), admfPeerCert(t))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if resp.Messages[0].ErrorInformation != nil {
		t.Fatalf("deactivation refused: %s", resp.Messages[0].ErrorInformation.ErrorDescription)
	}

	if n := sub.ids.Contexts(); n != 0 {
		t.Errorf("%d numbering contexts survive the tasking that created them", n)
	}
}

// TestTheTimestampIsTheEventsNotThePDUs is the property the whole timestamp
// decision rests on: what reaches the wire is the instant the caller observed the
// event, not the instant the PDU was assembled. Those differ whenever a record is
// built after the fact — a start-of-interception record for a UE that registered
// earlier, or several records built in a loop from one X1 activation — and a
// mediation function has no way to tell which one it was handed.
func TestTheTimestampIsTheEventsNotThePDUs(t *testing.T) {
	snd := &captureSender{}
	activateIRIWithTargets(t, snd, []types.TargetIdentifier{{Type: types.TargetSUPI, Value: testTargetSUPI}})
	sub := active.Load()

	happened := time.Date(2026, 8, 14, 6, 28, 15, 322_190_000, time.UTC)
	id := targetUE().IdentitySnapshot()
	sub.deliverIRI(sub.matchingTasks(id), targetsOf(id), happened, x2x3.DirectionFromTarget, amfRegistration(id, iri.RegResult3GPPAccess))
	sub.deliverIRI(sub.matchingTasks(id), targetsOf(id), happened, x2x3.DirectionFromTarget, amfRegistration(id, iri.RegResult3GPPAccess))

	if len(snd.pdus) != 2 {
		t.Fatalf("delivered %d records, want 2", len(snd.pdus))
	}
	for i, pdu := range snd.pdus {
		var got []byte
		for _, a := range pdu.Attributes {
			if a.Type == x2x3.AttrTimestamp {
				got = a.Value
			}
		}
		if len(got) != 8 {
			t.Fatalf("record %d carries no timestamp", i)
		}
		secs := binary.BigEndian.Uint32(got[0:4])
		nanos := binary.BigEndian.Uint32(got[4:8])
		if int64(secs) != happened.Unix() || int64(nanos) != int64(happened.Nanosecond()) {
			t.Errorf("record %d timestamped %d.%09d, want the event's %d.%09d — the clock was read where the PDU was built",
				i, secs, nanos, happened.Unix(), happened.Nanosecond())
		}
	}
}

// TestEveryIdentityThisPOIProducesRenders ties the invariant li/types asserts to what
// this element actually produces. targetsOf is the only source of the identities the
// header reports, so an identity it yields that cannot be rendered would cost the
// record a conditional attribute TS 33.128 marks required — with nothing failing.
//
// It covers the identities this fixture carries, which is every one targetsOf reads;
// the general rule (a subscriber identity type must have an element name)
// belongs to li/types, next to where such a type would be added.
func TestEveryIdentityThisPOIProducesRenders(t *testing.T) {
	ids := targetsOf(targetUE().IdentitySnapshot())
	if len(ids) != 4 {
		t.Fatalf("the fixture yields %d identities, want the SUPI, the PEI (as its IMEISV and the "+
			"IMEI it carries) and the GPSI this POI reads", len(ids))
	}
	for _, id := range ids {
		if _, ok := id.XMLFragment(); !ok {
			t.Errorf("%s renders no fragment, so a record matched on it would carry no matched target identifier", id.Type)
		}
	}
	if got := len(types.XMLFragments(ids)); got != len(ids) {
		t.Errorf("XMLFragments dropped %d of %d identities silently", len(ids)-got, len(ids))
	}
}

// modifyRequest builds a ModifyTaskRequest retargeting an XID to the given target
// SUPIs. A modification keeps the XID, which is the whole reason this element
// cannot treat one as an activation followed by a deactivation.
func modifyRequest(xid, admfID, neID string, supis ...string) []byte {
	var targets string
	for _, supi := range supis {
		targets += `<ns1:targetIdentifier><ns1:supiimsi>` + supi + `</ns1:supiimsi></ns1:targetIdentifier>`
	}

	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<ns1:X1Request xmlns:ns1="http://uri.etsi.org/03221/X1/2017/10" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <ns1:x1RequestMessage xsi:type="ns1:ModifyTaskRequest">
    <ns1:admfIdentifier>` + admfID + `</ns1:admfIdentifier>
    <ns1:neIdentifier>` + neID + `</ns1:neIdentifier>
    <ns1:messageTimestamp>2026-01-01T00:00:00.000000Z</ns1:messageTimestamp>
    <ns1:version>v1.6.1</ns1:version>
    <ns1:x1TransactionId>bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb</ns1:x1TransactionId>
    <ns1:taskDetails>
      <ns1:xId>` + xid + `</ns1:xId>
      <ns1:targetIdentifiers>` + targets + `</ns1:targetIdentifiers>
      <ns1:deliveryType>X2Only</ns1:deliveryType>
    </ns1:taskDetails>
  </ns1:x1RequestMessage>
</ns1:X1Request>`)
}

// TestModificationKeepsTheNumberingItsOwnRecordsUsed: the numbering state is keyed
// by the delivery XID, and a ModifyTask never changes the XID. Releasing it on a
// modification therefore discards contexts that are still in use — including ones
// the modification's own records have just consumed — and the next record in such
// a context repeats a number the mediation function has already seen for that
// (XID, Correlation ID) pair. A repeated number there is not a cosmetic defect: it
// is how a mediation function detects loss, so it must mean loss and nothing else.
func TestModificationKeepsTheNumberingItsOwnRecordsUsed(t *testing.T) {
	const xid, admf = "11111111-1111-4111-8111-111111111111", testADMFID

	snd := &captureSender{}
	st := store.New()
	st.Activate(types.InterceptTask{
		XID: xid,
		Targets: []types.TargetIdentifier{
			{Type: types.TargetSUPI, Value: testTargetSUPI},
			{Type: types.TargetGPSI, Value: "4915123456789"},
		},
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	})
	sub := &subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		neID: testNEID, ids: x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	}
	srv := newX1Server(st, Config{NEID: testNEID, AdmfID: admf}, sub)
	active.Store(sub)
	t.Cleanup(func() { active.Store(nil) })

	ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)
	if len(snd.pdus) != 1 || seqOf(t, snd.pdus[0]) != 0 {
		t.Fatalf("setup delivered %d records, first numbered %d", len(snd.pdus), seqOf(t, snd.pdus[0]))
	}

	// The warrant is narrowed to the subject's SUPI: the GPSI criterion goes and the
	// subject stays matched, so its numbering must continue with it.
	resp, err := srv.Process(modifyRequest(xid, admf, testNEID, testTargetSUPI), admfPeerCert(t))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if resp.Messages[0].ErrorInformation != nil {
		t.Fatalf("modification refused: %s", resp.Messages[0].ErrorInformation.ErrorDescription)
	}
	if n := sub.ids.Contexts(); n != 1 {
		t.Fatalf("the modification left %d numbering contexts, want the 1 still in use", n)
	}

	ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)
	if len(snd.pdus) != 2 {
		t.Fatalf("delivered %d records, want 2", len(snd.pdus))
	}
	if got := seqOf(t, snd.pdus[1]); got != 1 {
		t.Errorf("the record after the modification is numbered %d, want 1 — the modification "+
			"discarded numbering its subject's context was still using", got)
	}
}

// TestAnIMEIWarrantMatchesAUEThatPresentedItsIMEISV: an IMEISV is the IMEI's Type Allocation
// Code and Serial Number followed by a software version, so a UE that presented one is the
// device a warrant naming the fourteen-digit IMEI targets. Matching only the IMEISV as presented
// would leave that warrant producing nothing for a UE the AMF identified by IMEISV, which is
// every UE that did not offer a PEI before the AMF asked for one.
func TestAnIMEIWarrantMatchesAUEThatPresentedItsIMEISV(t *testing.T) {
	snd := &captureSender{}
	activateIRIWithTargets(t, snd, []types.TargetIdentifier{
		{Type: types.TargetPEI, Value: "35342500000001"},
	})

	ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)
	if len(snd.pdus) != 1 {
		t.Fatalf("delivered %d records for an IMEI warrant naming a UE that presented its IMEISV, want 1",
			len(snd.pdus))
	}

	attrs := attrsOf(snd.pdus[0])
	if got := attrs[x2x3.AttrMatchedTargetIdentifier]; len(got) != 1 || got[0] != derivedIMEIFragment {
		t.Errorf("matched target identifier = %q, want the IMEI the warrant named", got)
	}
}

// derivedIMEIFragment is the IMEI the test UE's IMEISV carries (its first fourteen digits), as
// the X2 target identifier attributes render it.
const derivedIMEIFragment = "<peiImei>35342500000001</peiImei>"

// TestAWarrantNamingTwoIdentitiesMatchesOnlyTheSubjectPresentingBoth is TS 103 221-1 table
// 6.2.1.2-1: "Where multiple criteria are present, all criteria are required to be
// matched." A warrant naming a SUPI and a GPSI describes the subscriber holding both; a UE
// presenting the SUPI with another GPSI is someone else, and reporting it would intercept
// a subject the warrant does not describe.
func TestAWarrantNamingTwoIdentitiesMatchesOnlyTheSubjectPresentingBoth(t *testing.T) {
	snd := &captureSender{}
	activateIRIWithTargets(t, snd, []types.TargetIdentifier{
		{Type: types.TargetSUPI, Value: testTargetSUPI},
		{Type: types.TargetGPSI, Value: "4915123456789"},
	})

	ReportRegistration(targetUE(), models.ACCESSTYPE__3_GPP_ACCESS)
	if len(snd.pdus) != 1 {
		t.Fatalf("the subject presenting both identities produced %d records, want 1", len(snd.pdus))
	}

	other := targetUE()
	other.Gpsi = "msisdn-4915100000000"
	ReportRegistration(other, models.ACCESSTYPE__3_GPP_ACCESS)
	if len(snd.pdus) != 1 {
		t.Errorf("a UE presenting only the warrant's SUPI produced %d more records, want none",
			len(snd.pdus)-1)
	}
}
