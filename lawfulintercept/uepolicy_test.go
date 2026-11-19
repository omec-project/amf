// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"bytes"
	"runtime"
	"strings"
	"testing"
	"time"
	"weak"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

// uePolicyCommandOf builds an n-octet MANAGE UE POLICY COMMAND under pti: the PTI, the message
// type, and a UE policy section management list (LV-E) filling the rest. Interior and trailing
// zero octets, the shapes a payload-mangling codec breaks.
func uePolicyCommandOf(pti uint8, n int) []byte {
	msg := make([]byte, n)
	msg[0], msg[1] = pti, uePolicyCommand
	listLen := n - 4
	msg[2], msg[3] = byte(listLen>>8), byte(listLen)
	for i := 4; i < n; i++ {
		msg[i] = byte(i * 37)
	}
	return msg
}

func uePolicyAnswer(pti, msgType uint8) []byte { return []byte{pti, msgType} }

// uePolicyRecords returns the uEPolicy of every AMFUEPolicyTransfer snd received, in order.
func uePolicyRecords(t *testing.T, snd *captureSender) [][]byte {
	t.Helper()

	var out [][]byte
	for _, rec := range decodeRecords(t, snd) {
		if rec.event == eventUEPolicyTransfer {
			out = append(out, rec.member(t, 6).Bytes)
		}
	}
	return out
}

// TestAUEPolicyCompleteReportsTheCommandItConfirms: TS 33.128 clause 6.2.2.2.12 triggers the
// record on the UE's MANAGE UE POLICY COMPLETE, and its uEPolicy is "Content of the N1 NAS
// message MANAGE UE POLICY COMMAND". One record, on the COMPLETE, carrying the COMMAND
// byte-identical.
func TestAUEPolicyCompleteReportsTheCommandItConfirms(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, testTargetSUPI)
	ue := targetUE()

	command := uePolicyCommandOf(5, 16)
	NoteUEPolicyCommand(ue, command)
	if n := len(snd.pdus); n != 0 {
		t.Fatalf("the COMMAND produced %d record(s); the clause triggers on the COMPLETE alone", n)
	}

	ReportUEPolicyComplete(ue, uePolicyAnswer(5, uePolicyComplete))

	got := uePolicyRecords(t, snd)
	if len(got) != 1 {
		t.Fatalf("delivered %d AMFUEPolicyTransfer records for one completed procedure, want 1", len(got))
	}
	if !bytes.Equal(got[0], command) {
		t.Errorf("uEPolicy = % x, want the COMMAND the COMPLETE confirms % x", got[0], command)
	}
}

// TestAUEPolicyCompleteWithNoCommandIsReportedNotProduced: the record's uEPolicy is mandatory
// and is the COMMAND; the COMPLETE is not that content. A COMPLETE whose COMMAND this element did
// not hold produces no record — not one carrying the two-octet COMPLETE, which the encoder
// refuses on every policy exchange as recordNotEncoded — and the warrant is told the record it
// mandates was not produced, rather than the loss being silent.
func TestAUEPolicyCompleteWithNoCommandIsReportedNotProduced(t *testing.T) {
	admf := newADMFStub(t)
	snd := &captureSender{}
	activateIRIReporting(t, snd, admf)

	ReportUEPolicyComplete(targetUE(), uePolicyAnswer(5, uePolicyComplete))

	if n := len(snd.pdus); n != 0 {
		t.Errorf("a COMPLETE with no COMMAND held produced %d record(s)", n)
	}
	got := admf.awaitReport(t, "recordNotProduced")
	if !strings.Contains(got, "recordNotProduced") {
		t.Errorf("a COMPLETE with no COMMAND held was not reported as recordNotProduced:\n%s", got)
	}
	// A refused record would be reported on its own goroutine, in no order relative to the
	// report above, so its absence is only shown after a wait.
	time.Sleep(time.Second)
	if got = strings.Join(admf.received(), "\n"); strings.Contains(got, "recordNotEncoded") {
		t.Errorf("a COMPLETE with no COMMAND held was encoded as a record and refused:\n%s", got)
	}
}

// TestAUEPolicyCommandAloneReportsNothing: a COMMAND the UE has not accepted is not the event
// the clause names.
func TestAUEPolicyCommandAloneReportsNothing(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, testTargetSUPI)

	NoteUEPolicyCommand(targetUE(), uePolicyCommandOf(5, 16))

	if n := len(snd.pdus); n != 0 {
		t.Errorf("an unanswered COMMAND produced %d record(s)", n)
	}
}

// TestInterleavedUEPolicyProceduresEachReportTheirOwnCommand: the COMPLETE names its procedure
// by PTI, and two procedures may be open at once. Each COMPLETE carries its own COMMAND.
func TestInterleavedUEPolicyProceduresEachReportTheirOwnCommand(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, testTargetSUPI)
	ue := targetUE()

	first, second := uePolicyCommandOf(1, 16), uePolicyCommandOf(2, 20)
	NoteUEPolicyCommand(ue, first)
	NoteUEPolicyCommand(ue, second)
	ReportUEPolicyComplete(ue, uePolicyAnswer(2, uePolicyComplete))
	ReportUEPolicyComplete(ue, uePolicyAnswer(1, uePolicyComplete))

	got := uePolicyRecords(t, snd)
	if len(got) != 2 || !bytes.Equal(got[0], second) || !bytes.Equal(got[1], first) {
		t.Errorf("records = % x, want the PTI 2 COMMAND then the PTI 1 COMMAND", got)
	}
}

// TestARetransmittedCommandIsHeldOnce: a T3501 retransmission repeats the COMMAND under its
// PTI. It replaces the one held rather than adding a second, so one COMPLETE is one record.
func TestARetransmittedCommandIsHeldOnce(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, testTargetSUPI)
	ue := targetUE()

	command := uePolicyCommandOf(7, 16)
	NoteUEPolicyCommand(ue, command)
	NoteUEPolicyCommand(ue, command)
	ReportUEPolicyComplete(ue, uePolicyAnswer(7, uePolicyComplete))

	if got := uePolicyRecords(t, snd); len(got) != 1 {
		t.Errorf("delivered %d records for one COMPLETE, want 1", len(got))
	}
}

// TestARetransmittedCompleteIsAnotherRecordNotALoss: a UE that receives a retransmitted COMMAND
// it has already answered retransmits its COMPLETE (TS 24.501 clause D.2.1.6 e), and TS 33.128
// clause 6.2.2.2.12 triggers on each N1MessageNotify carrying a COMPLETE. The second COMPLETE
// confirms the same COMMAND: it is a second record carrying it, not a recordNotProduced for a
// COMMAND the first COMPLETE used up.
func TestARetransmittedCompleteIsAnotherRecordNotALoss(t *testing.T) {
	admf := newADMFStub(t)
	snd := &captureSender{}
	activateIRIReporting(t, snd, admf)
	ue := targetUE()

	command := uePolicyCommandOf(7, 16)
	NoteUEPolicyCommand(ue, command)
	ReportUEPolicyComplete(ue, uePolicyAnswer(7, uePolicyComplete))
	ReportUEPolicyComplete(ue, uePolicyAnswer(7, uePolicyComplete))

	got := uePolicyRecords(t, snd)
	if len(got) != 2 || !bytes.Equal(got[0], command) || !bytes.Equal(got[1], command) {
		t.Errorf("records = % x, want the COMMAND twice", got)
	}
	time.Sleep(time.Second)
	if r := strings.Join(admf.received(), "\n"); strings.Contains(r, "recordNotProduced") {
		t.Errorf("a retransmitted COMPLETE was reported as a record not produced:\n%s", r)
	}
}

// TestARejectedUEPolicyCommandReportsNothing: the clause names only the COMPLETE. A COMMAND
// REJECT ends the procedure, so a later COMPLETE reusing the PTI does not report it either.
func TestARejectedUEPolicyCommandReportsNothing(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, testTargetSUPI)
	ue := targetUE()

	NoteUEPolicyCommand(ue, uePolicyCommandOf(3, 16))
	ReportUEPolicyComplete(ue, uePolicyAnswer(3, uePolicyCommandReject))
	ReportUEPolicyComplete(ue, uePolicyAnswer(3, uePolicyComplete))

	if n := len(snd.pdus); n != 0 {
		t.Errorf("a rejected COMMAND produced %d record(s)", n)
	}
}

// TestAnUntaskedUEHoldsNoUEPolicyCommand: a subscriber under no warrant costs nothing here.
func TestAnUntaskedUEHoldsNoUEPolicyCommand(t *testing.T) {
	activateIRI(t, &captureSender{}, "999999999999999")

	NoteUEPolicyCommand(targetUE(), uePolicyCommandOf(1, 16))

	if n := active.Load().policies.held(); n != 0 {
		t.Errorf("an untasked UE's COMMAND was held: %d UE(s) held", n)
	}
}

// TestDeregisteringFromEveryAccessForgetsTheUEPolicyCommands: a UE registered on no access
// completes no procedure it had open, so what was held for it goes.
func TestDeregisteringFromEveryAccessForgetsTheUEPolicyCommands(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, testTargetSUPI)
	ue := targetUE()

	NoteUEPolicyCommand(ue, uePolicyCommandOf(1, 16))
	ReportIdentifierDeassociation(ue)

	if n := active.Load().policies.held(); n != 0 {
		t.Errorf("%d UE(s) still held after the UE left every access", n)
	}
}

// TestHeldUEPolicyCommandsAreBounded: per UE, the oldest COMMAND gives way; in total, the UE
// whose newest COMMAND is oldest gives way to a new one.
func TestHeldUEPolicyCommandsAreBounded(t *testing.T) {
	var p pendingPolicies
	ue := &amfctx.AmfUe{}

	for pti := uint8(1); pti <= maxPendingPerUE+1; pti++ {
		p.remember(ue, pti, uePolicyCommandOf(pti, 16))
	}
	if n := len(p.byUE[weak.Make(ue)]); n != maxPendingPerUE {
		t.Errorf("one UE holds %d COMMANDs, want at most %d", n, maxPendingPerUE)
	}
	if p.lookup(ue, 1) != nil {
		t.Error("the oldest COMMAND survived the per-UE bound")
	}
	if p.lookup(ue, maxPendingPerUE+1) == nil {
		t.Error("the newest COMMAND was dropped instead of the oldest")
	}

	now := time.Unix(1_700_000_000, 0)
	p = pendingPolicies{now: func() time.Time { return now }}
	ues := make([]*amfctx.AmfUe, maxPendingUEs)
	for i := range ues {
		ues[i] = &amfctx.AmfUe{}
		p.remember(ues[i], 1, uePolicyCommandOf(1, 16))
		now = now.Add(time.Millisecond)
	}
	// The first UE answers its newest procedure last, so it is not the oldest.
	p.remember(ues[0], 2, uePolicyCommandOf(2, 16))
	now = now.Add(time.Millisecond)

	late := &amfctx.AmfUe{}
	p.remember(late, 1, uePolicyCommandOf(1, 16))
	if n := p.held(); n != maxPendingUEs {
		t.Errorf("%d UEs held, want %d", n, maxPendingUEs)
	}
	if p.lookup(late, 1) == nil {
		t.Error("a UE past the bound was not held: UEs that left unanswered block every later target")
	}
	if p.lookup(ues[1], 1) != nil {
		t.Error("the UE whose newest COMMAND was oldest was not the one evicted")
	}
	if p.lookup(ues[0], 2) == nil {
		t.Error("a UE with a recent COMMAND was evicted ahead of an older one")
	}
}

// TestVanishedUEsDoNotBlockAFreshTarget is the exhaustion case: as many targets as the bound
// are each sent a COMMAND and never answer, and their contexts are never removed. A target
// after them still has its exchange recorded. A bound that refused new entries would lose it,
// silently, until the AMF restarted.
func TestVanishedUEsDoNotBlockAFreshTarget(t *testing.T) {
	snd := &captureSender{}
	activateIRI(t, snd, testTargetSUPI)

	gone := make([]*amfctx.AmfUe, maxPendingUEs)
	for i := range gone {
		gone[i] = targetUE()
		NoteUEPolicyCommand(gone[i], uePolicyCommandOf(1, 16))
	}

	fresh := targetUE()
	command := uePolicyCommandOf(7, 20)
	NoteUEPolicyCommand(fresh, command)
	ReportUEPolicyComplete(fresh, uePolicyAnswer(7, uePolicyComplete))

	got := uePolicyRecords(t, snd)
	if len(got) != 1 || !bytes.Equal(got[0], command) {
		t.Fatalf("the fresh target produced %d record(s), want one carrying its COMMAND", len(got))
	}
	// Kept reachable so the bound is met by live UEs and the fresh one's room comes from
	// eviction, not from pruning collected entries.
	runtime.KeepAlive(gone)
}

// TestAnExpiredUEPolicyCommandIsNotReported: a COMMAND held past the procedure's own
// supervision belongs to a procedure the PCF has abandoned, so a COMPLETE under its PTI does
// not confirm it. An expired COMMAND is also dropped when the next one is held, so a UE that
// never answers does not hold it for the life of the context.
func TestAnExpiredUEPolicyCommandIsNotReported(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	p := pendingPolicies{now: func() time.Time { return now }}
	ue, other := &amfctx.AmfUe{}, &amfctx.AmfUe{}

	p.remember(ue, 1, uePolicyCommandOf(1, 16))
	now = now.Add(uePolicyHoldFor - time.Second)
	p.remember(ue, 2, uePolicyCommandOf(2, 16))
	if p.lookup(ue, 1) == nil {
		t.Fatal("a COMMAND inside its hold was not held")
	}

	now = now.Add(uePolicyHoldFor)
	if p.lookup(ue, 2) != nil {
		t.Error("a COMMAND past its hold was reported as confirmed")
	}

	p.remember(ue, 3, uePolicyCommandOf(3, 16))
	now = now.Add(uePolicyHoldFor)
	p.remember(other, 1, uePolicyCommandOf(1, 16))
	if n := p.held(); n != 1 {
		t.Errorf("%d UEs held after the first's COMMAND expired, want 1: an unanswered "+
			"COMMAND outlived its hold", n)
	}
	// Kept reachable so its entry goes by expiry, not by being collected.
	runtime.KeepAlive(ue)
}

// TestRemovingTheUEContextForgetsItsUEPolicyCommands: a context removed on any path —
// purge, transfer to another AMF, a registration torn down — completes no procedure through
// it. Forgetting only in the deregistration handlers would leave every other path's COMMANDs held.
func TestRemovingTheUEContextForgetsItsUEPolicyCommands(t *testing.T) {
	activateIRI(t, &captureSender{}, testTargetSUPI)
	ue := targetUE()

	NoteUEPolicyCommand(ue, uePolicyCommandOf(1, 16))
	if n := active.Load().policies.held(); n != 1 {
		t.Fatalf("%d UE(s) held before the removal, want 1", n)
	}

	ue.Remove()

	if n := active.Load().policies.held(); n != 0 {
		t.Errorf("%d UE(s) still held after the UE context was removed", n)
	}
}

// TestAShortUEPolicyCommandIsRefusedNotDelivered covers the one real conflict in the record's
// definition. TS 33.128 makes uEPolicy mandatory and defines `UEPolicy ::= OCTET STRING
// (SIZE(16..65540))`, which fits a COMMAND carrying the optional UE policy network classmark.
// A COMMAND with neither optional IE and a management list of 11–13 octets is 13–15 octets: the
// prose mandates the record and the schema forbids it. This element follows the schema and
// declares the contradiction (li/iri CONFORMANCE).
//
// What must not happen is that the gap is invisible. Relaxing the bound would not deliver the
// record — a mediation function validating against the published module discards it exactly
// as this encoder does — it would only move the discard to the far end, where this element
// cannot see it. So nothing is delivered, and the encoder's refusal is what reports it.
func TestAShortUEPolicyCommandIsRefusedNotDelivered(t *testing.T) {
	admf := newADMFStub(t)
	snd := &captureSender{}
	activateIRIReporting(t, snd, admf)
	ue := targetUE()

	NoteUEPolicyCommand(ue, uePolicyCommandOf(4, 13))
	ReportUEPolicyComplete(ue, uePolicyAnswer(4, uePolicyComplete))

	if n := len(snd.pdus); n != 0 {
		t.Errorf("delivered %d records for a COMMAND the definition forbids. A conformant "+
			"mediation function discards it, and this element would believe it had delivered", n)
	}
	got := awaitReport(t, admf, "recordNotEncoded")
	if !strings.Contains(got, "recordNotEncoded") {
		t.Errorf("the refused record was not reported against the warrant; the ADMF received:\n%s", got)
	}
	if strings.Contains(got, "recordNotProduced") {
		t.Errorf("the COMPLETE was not matched to its COMMAND, so this test is not exercising "+
			"the encoder's refusal; the ADMF received:\n%s", got)
	}
}

// TestOnlyACommandIsHeld: the network also sends UE POLICY PROVISIONING REJECT downlink
// (TS 24.501 table D.6.1.1). Held as a COMMAND, a COMPLETE reusing its PTI would report it.
func TestOnlyACommandIsHeld(t *testing.T) {
	activateIRI(t, &captureSender{}, testTargetSUPI)

	const uePolicyProvisioningReject = 0x06
	NoteUEPolicyCommand(targetUE(), []byte{1, uePolicyProvisioningReject, 0x00})

	if n := active.Load().policies.held(); n != 0 {
		t.Errorf("a downlink policy message other than a COMMAND was held: %d UE(s)", n)
	}
}

// TestAHeldCommandDoesNotKeepTheUEContextAlive: an entry is keyed by a weak pointer, so a UE
// context nothing else references is collected even while a COMMAND is held for it. Keyed by
// the pointer itself, every entry the bounds had not yet reached would pin a whole context.
func TestAHeldCommandDoesNotKeepTheUEContextAlive(t *testing.T) {
	var p pendingPolicies
	w := rememberForUnreferencedUE(&p)

	for i := 0; i < 10 && w.Value() != nil; i++ {
		runtime.GC()
	}
	if w.Value() != nil {
		t.Error("a held COMMAND kept its UE context reachable")
	}
	if n := p.held(); n != 1 {
		t.Fatalf("%d UE(s) held, want the one entry: the test did not hold a COMMAND", n)
	}
}

func rememberForUnreferencedUE(p *pendingPolicies) weak.Pointer[amfctx.AmfUe] {
	ue := &amfctx.AmfUe{}
	p.remember(ue, 1, uePolicyCommandOf(1, 16))
	return weak.Make(ue)
}

// TestACollectedUEDoesNotPushALiveOneOut: the entry of a UE context that has been collected
// can confirm nothing, so at the bound it gives up its slot before a live UE's COMMAND is
// evicted. Here the collected UE holds the newest COMMAND, so the eviction policy alone would
// pick the oldest live one.
func TestACollectedUEDoesNotPushALiveOneOut(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	p := pendingPolicies{now: func() time.Time { return now }}

	live := make([]*amfctx.AmfUe, maxPendingUEs-1)
	for i := range live {
		live[i] = &amfctx.AmfUe{}
		p.remember(live[i], 1, uePolicyCommandOf(1, 16))
		now = now.Add(time.Millisecond)
	}

	collected := func() weak.Pointer[amfctx.AmfUe] {
		ue := &amfctx.AmfUe{}
		p.remember(ue, 1, uePolicyCommandOf(1, 16))
		return weak.Make(ue)
	}()
	now = now.Add(time.Millisecond)
	for i := 0; i < 20 && collected.Value() != nil; i++ {
		runtime.GC()
	}
	if collected.Value() != nil {
		t.Fatal("the UE context was not collected, so this test cannot reach the state it is about")
	}

	p.remember(&amfctx.AmfUe{}, 1, uePolicyCommandOf(1, 16))

	if p.lookup(live[0], 1) == nil {
		t.Error("a live UE's COMMAND was evicted while a collected UE's entry held a slot")
	}
	runtime.KeepAlive(live)
}

// TestACommandIsHeldOnlyForATaskThatGetsItsRecord: a COMMAND is held so its COMPLETE can carry
// it, so only a task that is to receive AMFUEPolicyTransfer holds one. A task scoped to
// identifier association (TS 33.128 clause 6.2.2.2.1) or one that takes no IRI never receives
// the record, and a COMMAND held for it would only take a slot of the shared bound from a UE
// whose warrant does want it.
func TestACommandIsHeldOnlyForATaskThatGetsItsRecord(t *testing.T) {
	for _, c := range []struct {
		name     string
		scope    types.RecordScope
		products []types.ProductType
		wantHeld int
	}{
		{"identifier association only", types.RecordScopeIdentifierAssociation, []types.ProductType{types.ProductIRI}, 0},
		{"content only", types.RecordScopeStandard, []types.ProductType{types.ProductCC}, 0},
		{"no extension", types.RecordScopeStandard, []types.ProductType{types.ProductIRI}, 1},
		{"all", types.RecordScopeAll, []types.ProductType{types.ProductIRI}, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			st := store.New()
			if !st.Activate(types.InterceptTask{
				XID:         testSubstitutionXID,
				Targets:     []types.TargetIdentifier{{Type: types.TargetSUPI, Value: testTargetSUPI}},
				Products:    c.products,
				RecordScope: c.scope,
				State:       types.TaskActive,
			}) {
				t.Fatal("activate")
			}
			active.Store(&subsystem{
				store: st, senderFor: func(string) sender { return &captureSender{} },
				mdf2: testAgencyAddr,
				ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
			})
			t.Cleanup(func() { active.Store(nil) })

			NoteUEPolicyCommand(targetUE(), uePolicyCommandOf(1, 16))

			if n := active.Load().policies.held(); n != c.wantHeld {
				t.Errorf("%d UE(s) held a COMMAND, want %d", n, c.wantHeld)
			}
		})
	}
}
