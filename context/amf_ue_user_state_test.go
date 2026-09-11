// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"fmt"
	"sync"
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

// mutatedTac is the value a NAS or NGAP procedure writes after a reader has taken
// its copy. Named rather than repeated because goconst counts the literal across
// the package's tests.
const mutatedTac = "ffffff"

// TestAmfUeUserStateConcurrentAccess exercises the user-location and reachability
// fields from two goroutines at once — one writing them the way the NAS and NGAP
// procedures do, one reading them the way the Namf_EventExposure, Namf_Location,
// Namf_MT and OAM handlers do on their own HTTP goroutines. Run under `-race` it
// must pass.
//
// This is the regression test for the second AmfUe data race: #735 and #739 brought
// the six identity fields under identityMu, and the lock's own comment enumerated
// only those, so RatType, Location and Tai were left unguarded beside them.
// Replacing the accessor calls below with direct field access reproduces the
// failure.
func TestAmfUeUserStateConcurrentAccess(t *testing.T) {
	ue := &AmfUe{}
	const iterations = 500

	var wg sync.WaitGroup
	wg.Add(2)

	// Writer: models RanUe.UpdateLocation and the registration procedure.
	go func() {
		defer wg.Done()
		for i := range iterations {
			tac := fmt.Sprintf("%06x", i)
			ue.SetLocation(models.UserLocation{
				NrLocation: &models.NrLocation{Tai: models.Tai{Tac: tac}},
			})
			ue.SetTai(models.Tai{Tac: tac})
			ue.SetRatType(models.RATTYPE_NR)
		}
	}()

	// Reader: models a service handler assembling a response.
	go func() {
		defer wg.Done()
		for range iterations {
			location := ue.GetLocation()
			if location.NrLocation != nil {
				// Touch a member, not just the struct: a shallow copy would leave this
				// read aliasing the live state, which is the defect one level down.
				_ = location.NrLocation.Tai.Tac
			}
			_ = ue.GetTai().Tac
			_ = ue.GetRatType()
			_ = ue.GetReachability()
		}
	}()

	wg.Wait()
}

// TestSetLocationCopiesFromTheCaller pins that the setter takes ownership. Both of
// its callers pass a structure they keep using — RanUe.UpdateLocation passes
// ranUe.Location, which the NGAP handler goes on mutating — so storing the value as
// given would leave the UE's field aliasing state written without identityMu.
func TestSetLocationCopiesFromTheCaller(t *testing.T) {
	ue := &AmfUe{}
	source := models.UserLocation{
		NrLocation: &models.NrLocation{Tai: models.Tai{Tac: "000001"}},
	}
	ue.SetLocation(source)

	source.NrLocation.Tai.Tac = mutatedTac

	if got := ue.GetLocation().NrLocation.Tai.Tac; got != "000001" {
		t.Fatalf("stored location follows the caller's structure: Tac = %q, want %q", got, "000001")
	}
}

// TestGetLocationCopiesToTheConsumer pins the deep copy on the way out.
// models.UserLocation is five pointer-bearing members, so returning it under a read
// lock without copying hands the consumer a path into the live field: the read the
// value copy appears to end still happens later, through the members, on whatever
// goroutine serialises the response.
func TestGetLocationCopiesToTheConsumer(t *testing.T) {
	ue := &AmfUe{}
	ue.SetLocation(models.UserLocation{
		NrLocation: &models.NrLocation{Tai: models.Tai{Tac: "000001"}},
	})

	got := ue.GetLocation()

	// What a NAS or NGAP procedure does next. In-package direct access, because the
	// point is that no accessor is involved on this side.
	ue.Location.NrLocation.Tai.Tac = mutatedTac

	if got.NrLocation.Tai.Tac != "000001" {
		t.Fatalf("returned location aliases the live field: Tac = %q, want %q",
			got.NrLocation.Tai.Tac, "000001")
	}

	if first, second := ue.GetLocation(), ue.GetLocation(); first.NrLocation == second.NrLocation {
		t.Fatal("two reads share an NrLocation pointer, so the copy is shallow")
	}
}

// TestGetTaiCopiesTheNid covers the one pointer models.Tai holds. PlmnId is a value
// and Tac a string, so Nid is the only member a shallow copy would alias — and the
// one a reader would not think to look for.
func TestGetTaiCopiesTheNid(t *testing.T) {
	nid := "00000000001"
	ue := &AmfUe{}
	ue.SetTai(models.Tai{Tac: "000001", Nid: &nid})

	got := ue.GetTai()
	if got.Nid == nil {
		t.Fatal("Nid lost in the copy")
	}
	if got.Nid == ue.Tai.Nid {
		t.Fatal("returned Tai shares the live Nid pointer, so the copy is shallow")
	}
	if *got.Nid != nid {
		t.Fatalf("Nid = %q, want %q", *got.Nid, nid)
	}
}
