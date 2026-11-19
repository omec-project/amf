// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"sync"
	"testing"
	"time"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

// blockingSender holds each delivery until it is released, and is safe to read from
// another goroutine — which captureSender is not, and which matters because the
// start-of-interception scan runs on one.
type blockingSender struct {
	release chan struct{}

	mu   sync.Mutex
	pdus []*x2x3.PDU
}

func (b *blockingSender) Send(p *x2x3.PDU) error {
	<-b.release

	b.mu.Lock()
	defer b.mu.Unlock()
	b.pdus = append(b.pdus, p)

	return nil
}

func (b *blockingSender) count() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	return len(b.pdus)
}

// registeredUE puts a registered UE carrying supi into the AMF's pool.
func registeredUE(t *testing.T, supi string) {
	t.Helper()

	ue := &amfctx.AmfUe{Supi: "imsi-" + supi, Guti: testGUTI, Tmsi: 42}
	ue.State = map[models.AccessType]*fsm.State{
		models.ACCESSTYPE__3_GPP_ACCESS: fsm.NewState(amfctx.Registered),
	}
	amfctx.AMF_Self().UePool.Store(supi, ue)
	t.Cleanup(func() { amfctx.AMF_Self().UePool.Delete(supi) })
}

// TestStartOfInterceptionScanIsOffTheX1Goroutine.
//
// Activating a warrant makes the element walk every UE it holds. Done on the X1 request
// goroutine before answering, the time to acknowledge a tasking request would grow with
// the registered-UE population: an element that answers more slowly the busier it is,
// which is observable to whoever is asking, and is the provisioning-latency form of
// the same rule that keeps delivery off the signalling path. The SMF's equivalent runs
// its scan on a goroutine too.
//
// Asserted by blocking delivery. If the scan and its delivery were on the caller's
// goroutine, the call could not return until the sender was released — and the X1
// response would wait with it.
func TestStartOfInterceptionScanIsOffTheX1Goroutine(t *testing.T) {
	const supi = "262019876543210"

	release := make(chan struct{})
	snd := &blockingSender{release: release}

	task := types.InterceptTask{
		XID:      testSubstitutionXID,
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: supi}},
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	}
	st := store.New()
	if !st.Activate(task) {
		t.Fatal("activate")
	}
	s := &subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	}
	awaitScans(t, s)
	registeredUE(t, supi)

	returned := make(chan struct{})
	go func() {
		s.reportStartOfInterception(task, nil)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(3 * time.Second):
		t.Fatal("reportStartOfInterception did not return while delivery was blocked: " +
			"the UE-pool scan is still on the X1 request goroutine, so acknowledging a " +
			"warrant waits on every UE this element holds")
	}

	// And the work still happens — off the goroutine is not instead of.
	if n := snd.count(); n != 0 {
		t.Fatalf("%d records delivered before the sender was released", n)
	}
	close(release)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if snd.count() == 1 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Errorf("delivered %d start-of-interception records for one targeted registered UE, want 1",
		snd.count())
}

// TestStartOfInterceptionTimestampIsTheActivation keeps the instant where it belongs
// although the scan is handed off. The records report the *activation*, not when the
// scan happened to reach each UE, so the clock is read before the goroutine starts —
// a reading taken inside it would drift by however long the scan waited to be
// scheduled, and a mediation function cannot tell the two apart.
func TestStartOfInterceptionTimestampIsTheActivation(t *testing.T) {
	const supi = "262019876543211"

	release := make(chan struct{})
	close(release) // deliver immediately
	snd := &blockingSender{release: release}

	task := types.InterceptTask{
		XID:      "aaaaaaaa-0000-0000-0000-000000000002",
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: supi}},
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	}
	st := store.New()
	if !st.Activate(task) {
		t.Fatal("activate")
	}
	s := &subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	}
	awaitScans(t, s)
	registeredUE(t, supi)

	before := time.Now()
	s.reportStartOfInterception(task, nil)

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && snd.count() == 0 {
		time.Sleep(time.Millisecond)
	}
	after := time.Now()

	if snd.count() != 1 {
		t.Fatalf("delivered %d records, want 1", snd.count())
	}

	snd.mu.Lock()
	attrs := snd.pdus[0].Attributes
	snd.mu.Unlock()

	stamp, ok := timestampOf(attrs)
	if !ok {
		t.Fatal("the record carries no timestamp attribute")
	}
	if stamp.Before(before.Add(-time.Second)) || stamp.After(after) {
		t.Errorf("record timestamp %s is outside the activation window [%s, %s]", stamp, before, after)
	}
}

// timestampOf digs the clause 5.3.10 Timestamp out of a PDU's conditional
// attributes: two 32-bit unsigned integers, seconds then nanoseconds.
func timestampOf(attrs []x2x3.TLV) (time.Time, bool) {
	for _, a := range attrs {
		if a.Type == 9 && len(a.Value) == 8 {
			secs := uint32(a.Value[0])<<24 | uint32(a.Value[1])<<16 | uint32(a.Value[2])<<8 | uint32(a.Value[3])
			nsec := uint32(a.Value[4])<<24 | uint32(a.Value[5])<<16 | uint32(a.Value[6])<<8 | uint32(a.Value[7])

			return time.Unix(int64(secs), int64(nsec)), true
		}
	}

	return time.Time{}, false
}

// withdrawingSender withdraws the warrant as the first record is delivered, and counts
// what followed. Driven from the sender because a scan over a handful of UEs is over in
// microseconds: racing a goroutine at it would assert nothing.
type withdrawingSender struct {
	mu      sync.Mutex
	sent    int
	onFirst func()
}

func (w *withdrawingSender) Send(_ *x2x3.PDU) error {
	w.mu.Lock()
	w.sent++
	first := w.sent == 1
	onFirst := w.onFirst
	w.mu.Unlock()

	if first && onFirst != nil {
		onFirst()
	}

	return nil
}

func (w *withdrawingSender) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()

	return w.sent
}

// TestAWithdrawalDuringAScanStopsTheRemainingRecords is the authority half of the rule
// whose latency half TestStartOfInterceptionScanIsOffTheX1Goroutine pins, and the two
// are the same fact seen twice.
//
// The scan runs off the X1 goroutine because a provisioning answer must not scale with
// the registered-UE population. That is exactly what makes its duration unbounded by
// anything the provisioning function can see, so "the warrant was valid when this scan
// began" and "the warrant is valid now" stop being the same statement — and only the
// second one licenses a record. A DeactivateTask is acknowledged, the ADMF records the
// interception as ended, and records for it keep arriving at destinations that may
// already have been retired. Nothing outside the element can audit that.
func TestAWithdrawalDuringAScanStopsTheRemainingRecords(t *testing.T) {
	const supi = "262019876543210"

	task := types.InterceptTask{
		XID:      "aaaaaaaa-0000-0000-0000-000000000002",
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: supi}},
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	}
	st := store.New()
	if !st.Activate(task) {
		t.Fatal("activate")
	}

	snd := &withdrawingSender{onFirst: func() { st.Deactivate(task.XID) }}
	s := &subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	}
	awaitScans(t, s)

	// Several UEs one warrant covers, so there is a remainder for the withdrawal to
	// stop. They share a SUPI value the task targets and differ by pool key, which is
	// all this scan reads them for.
	for i := range 5 {
		ue := &amfctx.AmfUe{Supi: "imsi-" + supi, Guti: testGUTI, Tmsi: 42}
		ue.State = map[models.AccessType]*fsm.State{
			models.ACCESSTYPE__3_GPP_ACCESS: fsm.NewState(amfctx.Registered),
		}
		key := supi + "-" + string(rune('a'+i))
		amfctx.AMF_Self().UePool.Store(key, ue)
		t.Cleanup(func() { amfctx.AMF_Self().UePool.Delete(key) })
	}

	s.reportStartOfInterception(task, nil)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && snd.count() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond) // let any further records land

	if n := snd.count(); n == 0 {
		t.Fatal("the scan delivered nothing at all; this asserts nothing about withdrawal")
	} else if n > 1 {
		t.Errorf("%d records were delivered for a warrant withdrawn after the first; "+
			"the withdrawal was acknowledged and product kept arriving", n)
	}
}

// TestARetargetDuringAScanStopsRecordsForThePreviousSubject is the subject half of the same
// rule.
//
// The re-read answers that the warrant still exists, and — since the record is delivered
// under what the store now holds — which products it wants and where its product goes. It
// must also answer whether the warrant still names *this* subject: otherwise a ModifyTask
// that retargets a warrant mid-scan leaves the remaining UEs producing records about the
// previous subject, delivered under the warrant's own identifier to the new subject's agency:
// well-formed, correctly attributed, and about somebody the warrant no longer covers.
func TestARetargetDuringAScanStopsRecordsForThePreviousSubject(t *testing.T) {
	const (
		subject    = "262019876543210"
		retargeted = "262010000000001"
	)

	task := types.InterceptTask{
		XID:      "aaaaaaaa-0000-0000-0000-000000000003",
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: subject}},
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	}
	st := store.New()
	if !st.Activate(task) {
		t.Fatal("activate")
	}

	// The ADMF retargets the warrant as the first record goes out. Activate over a held
	// XID is how a modification reaches the store, which is the path x1's ModifyTask takes.
	retarget := task
	retarget.Targets = []types.TargetIdentifier{{Type: types.TargetSUPI, Value: retargeted}}
	snd := &withdrawingSender{onFirst: func() { st.Activate(retarget) }}

	s := &subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	}
	awaitScans(t, s)

	for i := range 5 {
		ue := &amfctx.AmfUe{Supi: "imsi-" + subject, Guti: testGUTI, Tmsi: 42}
		ue.State = map[models.AccessType]*fsm.State{
			models.ACCESSTYPE__3_GPP_ACCESS: fsm.NewState(amfctx.Registered),
		}
		key := subject + "-retarget-" + string(rune('a'+i))
		amfctx.AMF_Self().UePool.Store(key, ue)
		t.Cleanup(func() { amfctx.AMF_Self().UePool.Delete(key) })
	}

	s.reportStartOfInterception(task, nil)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && snd.count() == 0 {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond) // let any further records land

	if n := snd.count(); n == 0 {
		t.Fatal("the scan delivered nothing at all; this asserts nothing about retargeting")
	} else if n > 1 {
		t.Errorf("%d records were delivered for a subject the warrant no longer names: each is a "+
			"well-formed record about the wrong person, delivered under the warrant's own "+
			"identifier to the new subject's agency", n)
	}
}
