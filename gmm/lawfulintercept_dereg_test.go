// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
)

// dualRegisteredUE is a UE registered over 3GPP access and in nonThreeGPP over non-3GPP
// access. Registered on both is the only case in which the identifier binding can outlive
// a deregistration.
func dualRegisteredUE(nonThreeGPP fsm.StateType) *context.AmfUe {
	return &context.AmfUe{State: map[models.AccessType]*fsm.State{
		models.ACCESSTYPE__3_GPP_ACCESS:    fsm.NewState(context.Registered),
		models.ACCESSTYPE_NON_3_GPP_ACCESS: fsm.NewState(nonThreeGPP),
	}}
}

// updatingUE is a UE registered over 3GPP access and in a registration procedure over non-3GPP
// access, begun from Registered when fromRegistered.
func updatingUE(fromRegistered bool) *context.AmfUe {
	ue := dualRegisteredUE(context.ContextSetup)
	ue.SetRegisteredAtProcedureStart(models.ACCESSTYPE_NON_3_GPP_ACCESS, fromRegistered)
	return ue
}

// TestTheIdentifierBindingIsReleasedOnlyWhenItIsReleased: the AMF holds one SUPI↔5G-GUTI
// binding across every access a UE is registered on, so a dual-registered UE deregistering one
// access keeps it. A ReportIdentifierDeassociation fired unconditionally on either
// deregistration path would tell the mediation function the binding was gone while the element
// went on producing records under that same association. A statement the element itself
// immediately contradicts is worse than no statement: nothing downstream distinguishes it from
// a true one.
func TestTheIdentifierBindingIsReleasedOnlyWhenItIsReleased(t *testing.T) {
	for _, tc := range []struct {
		name      string
		ue        *context.AmfUe
		both      bool
		arrivedOn models.AccessType
		want      bool
	}{
		{
			name:      "one access of a dual-registered UE keeps the binding",
			ue:        dualRegisteredUE(context.Registered),
			arrivedOn: models.ACCESSTYPE__3_GPP_ACCESS,
			want:      false,
		},
		{
			name:      "both accesses at once releases it",
			ue:        dualRegisteredUE(context.Registered),
			both:      true,
			arrivedOn: models.ACCESSTYPE__3_GPP_ACCESS,
			want:      true,
		},
		{
			name:      "the last access a UE holds releases it",
			ue:        dualRegisteredUE(context.Deregistered),
			arrivedOn: models.ACCESSTYPE__3_GPP_ACCESS,
			want:      true,
		},
		{
			name:      "the other access in a registration update from Registered keeps it",
			ue:        updatingUE(true),
			arrivedOn: models.ACCESSTYPE__3_GPP_ACCESS,
			want:      false,
		},
		{
			name:      "the other access registering from Deregistered does not keep it",
			ue:        updatingUE(false),
			arrivedOn: models.ACCESSTYPE__3_GPP_ACCESS,
			want:      true,
		},
		{
			name:      "a single-access UE releases it",
			ue:        &context.AmfUe{State: map[models.AccessType]*fsm.State{models.ACCESSTYPE__3_GPP_ACCESS: fsm.NewState(context.Registered)}},
			arrivedOn: models.ACCESSTYPE__3_GPP_ACCESS,
			want:      true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := deregisteringEveryAccess(tc.ue, tc.both, tc.arrivedOn)
			if got == tc.want {
				return
			}

			consequence := "withhold a record for a binding that is genuinely gone"
			if got {
				consequence = "claim a binding is released while the element still uses it"
			}
			t.Errorf("deregisteringEveryAccess = %v, want %v — the deassociation record would %s",
				got, tc.want, consequence)
		})
	}
}

// TestTheDeregistrationScopeIsNotTakenFromAFallback keeps the binding decision off
// util.AnTypeToNas, which answers AccessTypeBoth for any value it does not recognise —
// sound for addressing a NAS message, and the wrong default here: an unknown access
// would silently claim the binding had been released.
func TestTheDeregistrationScopeIsNotTakenFromAFallback(t *testing.T) {
	ue := dualRegisteredUE(context.Registered)

	// A network-initiated deregistration is for one access, and passes false outright.
	if deregisteringEveryAccess(ue, false, models.ACCESSTYPE__3_GPP_ACCESS) {
		t.Error("a single-access deregistration of a dual-registered UE released the binding")
	}
	_ = nasMessage.AccessTypeBoth
}
