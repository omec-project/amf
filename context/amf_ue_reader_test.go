// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"runtime"
	"sync"
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

// The writers of these maps take ue.Mutex, but a guarded write still races an unguarded
// read: Go's fatal "concurrent map read and map write" fires on the pair, not on the
// side that holds the lock. Readers run on SBI handlers and on the NGAP reader goroutine
// while the UE's own procedures write, so both sides have to go through the lock.
//
// Under -race this fails if any accessor stops taking it.
func TestReadingAContextWhileEveryMapIsWritten(t *testing.T) {
	// The detector needs more than one P to see any of this: with a lock deliberately
	// removed from GetReleaseCause, GOMAXPROCS=1 reported no races at all. A runner with one
	// CPU would otherwise pass this test against a broken accessor, so the test asks for
	// what it needs rather than depending on the machine it lands on.
	if previous := runtime.GOMAXPROCS(0); previous < 2 {
		runtime.GOMAXPROCS(2)

		t.Cleanup(func() { runtime.GOMAXPROCS(previous) })
	}

	ue := &AmfUe{}
	ue.init()
	ue.Supi = testSupi

	ran := &AmfRan{AnType: models.ACCESSTYPE__3_GPP_ACCESS, GnbId: testGnbId}
	anType := models.ACCESSTYPE__3_GPP_ACCESS

	const rounds = 2000

	writers := []func(i int){
		func(i int) { ue.SetAllowedNssai(anType, []models.AllowedSnssai{{}}) },
		func(i int) { ue.AppendAllowedNssai(anType, models.AllowedSnssai{}) },
		func(i int) { ue.SetRegistrationArea(anType, nil) },
		func(i int) { ue.AppendRegistrationArea(anType, models.Tai{}) },
		func(i int) { ue.SetReleaseCause(anType, &CauseAll{}) },
		func(i int) {
			remaining := int32(i)
			ue.SetEventSubscription("sub", &AmfUeEventSubscription{RemainReports: &remaining})
		},
		func(i int) { ue.DecrementRemainReports("sub") },
		func(i int) { ue.DeleteEventSubscription("sub") },
		func(i int) { ue.SetOnGoing(anType, &OnGoingProcedureWithPrio{Procedure: OnGoingProcedureNothing}) },
		func(i int) { ue.AttachRanUe(&RanUe{RanUeNgapId: int64(i), AmfUeNgapId: int64(i), Ran: ran}) },
		func(i int) { ue.DetachRanUe(anType) },
		func(i int) { ue.ClearRegistrationRequestData(anType) },
	}

	readers := []func(){
		func() {
			// The range exercises the returned slice. Note this does not demonstrate the
			// copy is necessary: no writer here mutates an existing element, so a
			// returned slice would not be written under this loop. What it does cover is
			// the map access inside the accessor.
			for range ue.GetAllowedNssai(anType) { //nolint:revive // the range is the test
			}
		},
		func() { _ = ue.AllowedNssaiLen(anType) },
		func() {
			for range ue.GetRegistrationArea(anType) { //nolint:revive // the range is the test
			}
		},
		func() { _ = ue.RegistrationAreaLen(anType) },
		func() { _, _ = ue.GetReleaseCause(anType) },
		func() { _, _ = ue.GetEventSubscription("sub") },
		func() {
			for range ue.GetEventSubscriptions() { //nolint:revive // the range is the test
			}
		},
		func() { _ = ue.GetOnGoing(anType) },
		func() { _ = ue.GetRanUe(anType) },
		func() { _ = ue.InAllowedNssai(models.Snssai{}, anType) },
		func() { _ = ue.GetNsiInformationFromSnssai(anType, models.Snssai{}) },
		func() { _ = ue.TaiListInRegistrationArea([]models.Tai{{}}, anType) },
	}

	var wg sync.WaitGroup

	for _, write := range writers {
		wg.Add(1)

		go func(write func(int)) {
			defer wg.Done()

			for i := range rounds {
				write(i)
			}
		}(write)
	}

	for _, read := range readers {
		wg.Add(1)

		go func(read func()) {
			defer wg.Done()

			for range rounds {
				read()
			}
		}(read)
	}

	wg.Wait()
}

// A subscription handed out under the lock and then read outside it is read through the
// same pointer the encoder walks when the context is persisted. The counter is copied, so
// what a caller reads afterwards is its own.
func TestGetEventSubscriptionReturnsItsOwnCounter(t *testing.T) {
	ue := &AmfUe{}
	ue.init()

	remaining := int32(3)
	ue.SetEventSubscription("sub", &AmfUeEventSubscription{RemainReports: &remaining})

	taken, ok := ue.GetEventSubscription("sub")
	if !ok || taken.RemainReports == nil {
		t.Fatalf("precondition: the subscription and its counter must come back")
	}

	*taken.RemainReports = 99

	again, _ := ue.GetEventSubscription("sub")
	if got := *again.RemainReports; got != 3 {
		t.Errorf("the stored counter = %d after a caller wrote its copy, want 3: the caller was handed the live one", got)
	}
}

// The decrement is a read-modify-write on a counter the encoder reads under ue.Mutex, so it
// belongs to the context rather than to the caller holding a pointer into it.
func TestDecrementRemainReportsTakesOneOff(t *testing.T) {
	ue := &AmfUe{}
	ue.init()

	remaining := int32(2)
	ue.SetEventSubscription("sub", &AmfUeEventSubscription{RemainReports: &remaining})

	ue.DecrementRemainReports("sub")

	got, ok := ue.GetEventSubscription("sub")
	if !ok || got.RemainReports == nil {
		t.Fatalf("the subscription lost its counter")
	}

	if *got.RemainReports != 1 {
		t.Errorf("reports left = %d after one report, want 1", *got.RemainReports)
	}
}

// A subscription that has no counter is not a reason to end the process, and neither is one
// that has been deleted between a report being built and its counter being taken down.
func TestDecrementRemainReportsToleratesWhatIsNotThere(t *testing.T) {
	ue := &AmfUe{}
	ue.init()

	ue.DecrementRemainReports("never-set")

	ue.SetEventSubscription("sub", &AmfUeEventSubscription{})
	ue.DecrementRemainReports("sub")
}
