// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"sync"
	"testing"
	"time"

	amfctx "github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"github.com/omec-project/util/li/store"
	"github.com/omec-project/util/li/types"
	"github.com/omec-project/util/li/x2x3"
)

// registeredTargets puts n registered UEs one warrant covers into the pool.
func registeredTargets(t *testing.T, supi string, n int, keyPrefix string) {
	t.Helper()

	for i := range n {
		ue := &amfctx.AmfUe{Supi: "imsi-" + supi}
		ue.State = map[models.AccessType]*fsm.State{
			models.ACCESSTYPE__3_GPP_ACCESS: fsm.NewState(amfctx.Registered),
		}
		key := keyPrefix + "-" + string(rune('a'+i))
		amfctx.AMF_Self().UePool.Store(key, ue)
		t.Cleanup(func() { amfctx.AMF_Self().UePool.Delete(key) })
	}
}

// countingScanFixture is a subsystem whose deliveries are counted.
func countingScanFixture(t *testing.T, task types.InterceptTask) (*subsystem, *withdrawingSender) {
	t.Helper()

	st := store.New()
	if !st.Activate(task) {
		t.Fatal("activate")
	}
	snd := &withdrawingSender{}
	s := &subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	}
	awaitScans(t, s)

	return s, snd
}

// TestAModificationThatCannotProduceARecordStartsNoScan is the cost half of the scan.
//
// Each scan launches a goroutine that walks the whole registered-UE pool. For a modification
// that changed only the destinations, the record scope or the delivery labelling, `covered()`
// suppresses every record the walk produces — because a UE the previous task already covered
// is not one whose interception begins here, and for those modifications that is every UE. So
// the walk is pure cost, and bulk provisioning runs one per warrant concurrently, on an
// element whose ordinary job is answering NAS procedures for those same UEs.
//
// The predicate is deliberately three equalities rather than an attempt to be clever: getting
// it too narrow loses a record that is due, and getting it too wide only costs a walk.
func TestAModificationThatCannotProduceARecordStartsNoScan(t *testing.T) {
	const supi = "262019876543210"

	base := types.InterceptTask{
		XID:      "aaaaaaaa-0000-0000-0000-000000000010",
		Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: supi}},
		Products: []types.ProductType{types.ProductIRI},
		State:    types.TaskActive,
	}

	for _, tc := range []struct {
		name     string
		change   func(types.InterceptTask) types.InterceptTask
		wantScan bool
	}{
		{"destinations only", func(t types.InterceptTask) types.InterceptTask {
			t.DIDs = []string{testOtherDID}

			return t
		}, false},
		{"delivery label only", func(t types.InterceptTask) types.InterceptTask {
			t.ProductID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"

			return t
		}, false},
		{"a product added", func(t types.InterceptTask) types.InterceptTask {
			t.Products = []types.ProductType{types.ProductIRI, types.ProductCC}

			return t
		}, true},
		{"the record scope widened", func(t types.InterceptTask) types.InterceptTask {
			t.RecordScope = types.RecordScopeAll

			return t
		}, true},
		{"a target replaced", func(t types.InterceptTask) types.InterceptTask {
			t.Targets = []types.TargetIdentifier{{Type: types.TargetSUPI, Value: "262010000000009"}}

			return t
		}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _ := countingScanFixture(t, base)
			registeredTargets(t, supi, 3, "predicate-"+tc.name)

			// The walk is what is being counted, not the records: a modification that cannot
			// produce a record could still walk the pool and have `covered()` throw everything
			// away.
			var walks int
			var mu sync.Mutex
			restore := scanWalked
			scanWalked = func() {
				mu.Lock()
				defer mu.Unlock()
				walks++
			}
			t.Cleanup(func() { scanWalked = restore })

			next := tc.change(base)
			s.applyTaskChange(&base, &next)
			s.scans.Wait()

			mu.Lock()
			got := walks
			mu.Unlock()

			if tc.wantScan && got == 0 {
				t.Error("a modification that can begin interception of a product or a subject " +
					"started no scan: the record that would have said so is never produced")
			}
			if !tc.wantScan && got != 0 {
				t.Errorf("a modification that cannot produce a record walked the UE pool %d times: "+
					"every record it produced is suppressed by covered(), so the walk is pure cost "+
					"— and bulk provisioning runs one per warrant at once", got)
			}
		})
	}
}

// TestConcurrentActivationScansAreBounded is the other half: the scans that do survive the
// predicate cost a queue rather than one full pool walk each.
func TestConcurrentActivationScansAreBounded(t *testing.T) {
	const supi = "262019876543210"

	registeredTargets(t, supi, 2, "bounded")

	// Each walk blocks until released, so the peak is observable.
	release := make(chan struct{})
	var (
		mu       sync.Mutex
		inFlight int
		peak     int
	)
	restore := scanWalked
	scanWalked = func() {
		mu.Lock()
		inFlight++
		if inFlight > peak {
			peak = inFlight
		}
		mu.Unlock()

		<-release

		mu.Lock()
		inFlight--
		mu.Unlock()
	}
	t.Cleanup(func() { scanWalked = restore })

	st := store.New()
	snd := &withdrawingSender{}
	s := &subsystem{
		store: st, senderFor: func(string) sender { return snd },
		mdf2: testAgencyAddr,
		ids:  x2x3.NewIdentity(testNEID, amfInterceptionPoint),
	}
	awaitScans(t, s)

	// Bulk provisioning: many warrants at once, which is what TS 103 221-1's bulk operations
	// and an ADMF restoring tasking after a restart both look like.
	const warrants = 16
	for i := range warrants {
		task := types.InterceptTask{
			XID:      types.XID("aaaaaaaa-0000-0000-0000-0000000000" + string(rune('a'+i))),
			Targets:  []types.TargetIdentifier{{Type: types.TargetSUPI, Value: supi}},
			Products: []types.ProductType{types.ProductIRI},
			State:    types.TaskActive,
		}
		s.applyTaskChange(nil, &task)
	}

	// Let them pile up against the bound.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		reached := peak
		mu.Unlock()
		if reached >= cap(scanSlots) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(release)
	s.scans.Wait()

	mu.Lock()
	reached := peak
	mu.Unlock()

	if reached > cap(scanSlots) {
		t.Errorf("%d scans walked the UE pool at once against a bound of %d: bulk provisioning "+
			"costs one full walk per warrant simultaneously, on an element whose ordinary job is "+
			"answering NAS procedures for those same UEs", reached, cap(scanSlots))
	}
	if reached == 0 {
		t.Error("no scan ran at all; this test asserts nothing")
	}
}

var _ = nasMessage.RegistrationType5GSInitialRegistration
