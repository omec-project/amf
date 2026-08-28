// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
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
	ue := &AmfUe{}
	ue.init()
	ue.Supi = testSupi

	ran := &AmfRan{AnType: models.ACCESSTYPE__3_GPP_ACCESS, GnbId: "208:93:00100c"}
	anType := models.ACCESSTYPE__3_GPP_ACCESS

	const rounds = 2000

	writers := []func(i int){
		func(i int) { ue.SetAllowedNssai(anType, []models.AllowedSnssai{{}}) },
		func(i int) { ue.AppendAllowedNssai(anType, models.AllowedSnssai{}) },
		func(i int) { ue.SetRegistrationArea(anType, nil) },
		func(i int) { ue.AppendRegistrationArea(anType, models.Tai{}) },
		func(i int) { ue.SetReleaseCause(anType, &CauseAll{}) },
		func(i int) { ue.SetEventSubscription("sub", &AmfUeEventSubscription{}) },
		func(i int) { ue.DeleteEventSubscription("sub") },
		func(i int) { ue.SetOnGoing(anType, &OnGoingProcedureWithPrio{Procedure: OnGoingProcedureNothing}) },
		func(i int) { ue.AttachRanUe(&RanUe{RanUeNgapId: int64(i), AmfUeNgapId: int64(i), Ran: ran}) },
		func(i int) { ue.DetachRanUe(anType) },
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
		func() { _ = ue.GetOnGoing(anType) },
		func() { _ = ue.GetRanUe(anType) },
		func() { _ = ue.InAllowedNssai(models.Snssai{}, anType) },
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
