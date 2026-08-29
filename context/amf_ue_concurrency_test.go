// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"sync"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/omec-project/openapi/v2/models"
)

// testSupi is the subscriber every test in this package builds its UE around, and
// testGnbId the RAN it attaches to.
const (
	testSupi  = "imsi-208930100007487"
	testGnbId = "208:93:00100c"
)

// Persisting a UE context marshals six maps that other goroutines write while the
// UE is being served: RanUe, OnGoing, RegistrationArea, AllowedNssai, ReleaseCause
// and EventSubscriptionsInfo. A write landing during that marshal is a fatal runtime
// error -- "concurrent map read and map write" -- which ends the process and every
// UE on this AMF, not the one procedure that happened to be running.
//
// Both sides go through ue.Mutex, so this drives every writer against a marshalling
// goroutine. Under -race it fails if any of them stops taking the lock.
func TestStoringAContextWhileEveryMapIsWritten(t *testing.T) {
	ue := &AmfUe{}
	ue.init()
	ue.Supi = testSupi

	ran := &AmfRan{AnType: models.ACCESSTYPE__3_GPP_ACCESS, GnbId: testGnbId}
	anType := models.ACCESSTYPE__3_GPP_ACCESS

	const rounds = 2000

	writers := []func(i int){
		func(i int) { ue.SetAllowedNssai(anType, []models.AllowedSnssai{{}}) },
		func(i int) { ue.AppendAllowedNssai(anType, models.AllowedSnssai{}) },
		func(i int) { ue.SetOnGoing(anType, &OnGoingProcedureWithPrio{Procedure: OnGoingProcedureNothing}) },
		func(i int) { ue.SetReleaseCause(anType, &CauseAll{}) },
		func(i int) { ue.SetRegistrationArea(anType, nil) },
		func(i int) { ue.AppendRegistrationArea(anType, models.Tai{}) },
		func(i int) { ue.SetEventSubscription("sub", &AmfUeEventSubscription{}) },
		func(i int) { ue.DeleteEventSubscription("sub") },
		func(i int) {
			ranUe := &RanUe{RanUeNgapId: int64(i), AmfUeNgapId: int64(i), Ran: ran}
			ue.AttachRanUe(ranUe)
		},
		func(i int) { ue.DetachRanUe(anType) },
		func(i int) { _ = ue.RegistrationAreaLen(anType) },
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

	wg.Add(1)

	go func() {
		defer wg.Done()

		for range rounds {
			if _, err := sonic.Marshal(ue); err != nil {
				t.Errorf("storing a context must not fail: %v", err)
				return
			}
		}
	}()

	wg.Wait()
}
