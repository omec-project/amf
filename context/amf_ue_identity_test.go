// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"sync"
	"testing"
)

// TestAmfUeIdentityConcurrentAccess exercises the AmfUe identity fields from two
// goroutines at once — one writing them the way a NAS procedure does, one reading
// a consistent snapshot the way an off-NAS consumer does (an SBI handler, logging,
// or the LI start-of-interception scan). Run under `-race` it must pass.
//
// This is the regression test for the AmfUe identity data race: before the Get*/
// Set*/IdentitySnapshot accessors, both sides touched the fields directly (e.g.
// `ue.Supi = …` on the NAS goroutine and `_ = ue.Supi` on another), which the race
// detector flags — and a torn read of a string field is a genuine memory-safety
// hazard, not just stale data. Replacing the accessor calls below with direct
// field access reproduces the failure.
func TestAmfUeIdentityConcurrentAccess(t *testing.T) {
	ue := &AmfUe{}
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: models the NAS procedure assigning identity fields during
	// registration / GUTI reallocation.
	go func() {
		defer wg.Done()
		for i := range iterations {
			ue.SetSupi(fmt.Sprintf("imsi-%015d", i))
			ue.SetPei(fmt.Sprintf("imeisv-%016d", i))
			ue.SetGpsi(fmt.Sprintf("msisdn-%d", i))
			ue.SetGuti(fmt.Sprintf("guti-%d", i))
			ue.SetTmsi(int32(i))
			ue.SetRegistrationType5GS(uint8(i % 4))
		}
	}()

	// Reader: models a consumer on another goroutine reading the UE's identity.
	go func() {
		defer wg.Done()
		for range iterations {
			id := ue.IdentitySnapshot()
			// Touch every field so the race detector observes the reads.
			_ = id.Supi + id.Pei + id.Gpsi + id.Guti
			_ = id.Tmsi
			_ = id.RegistrationType5GS
			// The individual getters must be safe too.
			_ = ue.GetSupi()
			_ = ue.GetTmsi()
			_ = ue.GetRegistrationType5GS()
		}
	}()

	wg.Wait()
}
