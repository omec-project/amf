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

// Persisting a UE context marshals maps that other goroutines write: AttachRanUe and
// DetachRanUe hold ue.Mutex while changing ue.RanUe, and the encoder iterates it. If
// the marshal does not take the same lock, the runtime aborts the whole process with
// "concurrent map iteration and map write" -- not one failed store, every UE on this
// AMF. Under -race this test fails if MarshalJSON stops holding the lock.
func TestStoringAContextWhileItsRanUeChanges(t *testing.T) {
	ue := &AmfUe{}
	ue.init()
	ue.Supi = testSupi

	ran := &AmfRan{AnType: models.ACCESSTYPE__3_GPP_ACCESS, GnbId: "208:93:00100c"}

	const rounds = 3000

	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()

		for i := range rounds {
			// What AttachRanUe and DetachRanUe do to the map, under the same lock.
			ranUe := &RanUe{RanUeNgapId: int64(i), AmfUeNgapId: int64(i), Ran: ran}

			ue.Mutex.Lock()
			ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] = ranUe
			ue.Mutex.Unlock()

			ue.Mutex.Lock()
			delete(ue.RanUe, models.ACCESSTYPE__3_GPP_ACCESS)
			ue.Mutex.Unlock()
		}
	}()

	wg.Add(1)

	go func() {
		defer wg.Done()

		for range rounds {
			if _, err := sonic.Marshal(ue); err != nil {
				t.Errorf("marshalling a context must not fail: %v", err)
				return
			}
		}
	}()

	wg.Wait()
}
