// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

// TestADeregistered3GPPAccessReportsNoRegistrationArea: a UE registered over both accesses
// that deregisters only 3GPP access stays in the pool for its non-3GPP registration, and
// the 3GPP registration area allocated for the registration it left is not cleared. The
// interception records that follow — start of interception, a non-3GPP registration — take
// fiveGSTAIList from the snapshot, which must then report no area rather than the one the UE
// left.
func TestADeregistered3GPPAccessReportsNoRegistrationArea(t *testing.T) {
	ue := &AmfUe{}
	ue.init()
	ue.Supi = testSupi

	area := []models.Tai{{PlmnId: models.PlmnId{Mcc: "208", Mnc: "93"}, Tac: "000001"}}
	ue.SetRegistrationArea(models.ACCESSTYPE__3_GPP_ACCESS, area)

	// A registration under way: the records of the registration report the area it
	// allocated, before the access reaches Registered.
	ue.State[models.ACCESSTYPE__3_GPP_ACCESS].Set(ContextSetup)
	if got := ue.IdentitySnapshot().TaiList; len(got) != 1 {
		t.Fatalf("during the registration the snapshot reports %v, want the area it allocated", got)
	}

	// Registered over both accesses.
	ue.State[models.ACCESSTYPE__3_GPP_ACCESS].Set(Registered)
	ue.State[models.ACCESSTYPE_NON_3_GPP_ACCESS].Set(Registered)
	if got := ue.IdentitySnapshot().TaiList; len(got) != 1 {
		t.Fatalf("registered over 3GPP access the snapshot reports %v, want its area", got)
	}

	// 3GPP access only deregisters, as SetDeregisteredState does for AccessType3GPP; the
	// area stays in the context.
	ue.State[models.ACCESSTYPE__3_GPP_ACCESS].Set(Deregistered)
	if n := ue.RegistrationAreaLen(models.ACCESSTYPE__3_GPP_ACCESS); n != 1 {
		t.Fatalf("the context holds %d 3GPP TAIs; the test assumes deregistration keeps them", n)
	}
	if got := ue.IdentitySnapshot().TaiList; got != nil {
		t.Errorf("with 3GPP access deregistered and non-3GPP access still registered the "+
			"snapshot reports %v as the registration area, want none: the UE is no longer "+
			"registered in it", got)
	}
}
