// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	ctxt "context"
	"slices"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
)

// releaseCompletePdu builds a UE Context Release Complete naming the given RanUe,
// the same shape the existing release tests in this package use.
func releaseCompletePdu(ranUe *context.RanUe) *ngapType.NGAPPDU {
	return &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentSuccessfulOutcome,
		SuccessfulOutcome: &ngapType.SuccessfulOutcome{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeUEContextRelease},
			Value: ngapType.SuccessfulOutcomeValue{
				Present: ngapType.SuccessfulOutcomePresentUEContextRelease,
				UEContextRelease: &ngapType.UEContextReleaseComplete{
					ProtocolIEs: ngapType.ProtocolIEContainerUEContextReleaseCompleteIEs{
						List: []ngapType.UEContextReleaseCompleteIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Value: ngapType.UEContextReleaseCompleteIEsValue{
									Present:     ngapType.UEContextReleaseCompleteIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: ranUe.AmfUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Value: ngapType.UEContextReleaseCompleteIEsValue{
									Present:     ngapType.UEContextReleaseCompleteIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUe.RanUeNgapId},
								},
							},
						},
					},
				},
			},
		},
	}
}

// newReleasingUe returns a UE holding a valid security context, attached to a RanUe
// whose release is about to complete with the given action.
func newReleasingUe(t *testing.T, supi string, action context.RelAction, ngapId int64) (
	*context.AmfRan, *context.RanUe, *context.AmfUe,
) {
	t.Helper()
	self := context.AMF_Self()
	ran := context.NewAmfRanDefault()
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS

	amfUe := self.NewAmfUe(supi)
	// The whole point of the distinction under test: this UE *has* a usable security
	// context, which is the case that keeps a stored context on a normal release.
	amfUe.SecurityContextAvailable = true

	ranUe, err := ran.NewRanUe(ngapId)
	if err != nil {
		t.Fatalf("creating RanUe: %v", err)
	}
	ranUe.Log = logger.NgapLog
	ranUe.ReleaseAction = action
	amfUe.AttachRanUe(ranUe)

	t.Cleanup(func() {
		if leftover := self.RanUeFindByAmfUeNgapIDLocal(ranUe.AmfUeNgapId); leftover != nil {
			if err := leftover.Remove(); err != nil {
				t.Logf("removing the leftover RanUe: %v", err)
			}
		}
		self.UePool.Delete(supi)
	})
	return ran, ranUe, amfUe
}

// recordDatastoreWrites replaces the deregistration release's datastore writes for the test
// and returns the writes made, as "store <supi>" or "delete <supi>".
func recordDatastoreWrites(t *testing.T) *[]string {
	t.Helper()

	originalStore, originalDelete := storeContextInDB, deleteContextFromDB
	t.Cleanup(func() { storeContextInDB, deleteContextFromDB = originalStore, originalDelete })

	writes := &[]string{}
	storeContextInDB = func(ue *context.AmfUe) { *writes = append(*writes, "store "+ue.GetSupi()) }
	deleteContextFromDB = func(ue *context.AmfUe) { *writes = append(*writes, "delete "+ue.GetSupi()) }

	return writes
}

// A UE that deregisters has left the network, so the context the AMF holds for it is
// not a head start for a later registration -- it is state that the next registration
// will find and have to reconcile. Asserted against the normal-release case in the
// same test, because the two differ only in the release action and it is precisely
// that difference the fix introduces: asserting the deregistration half alone would
// still pass if the action were ignored and everything were deleted.
func TestUeInitiatedDeregistrationRemovesUeDespiteSecurityContext(t *testing.T) {
	self := context.AMF_Self()

	const deregSupi = "imsi-208930000000101"
	deregRan, deregRanUe, _ := newReleasingUe(
		t, deregSupi, context.UeContextReleaseDueToUeInitiatedDeregistration, 101)

	const keepSupi = "imsi-208930000000102"
	keepRan, keepRanUe, _ := newReleasingUe(
		t, keepSupi, context.UeContextReleaseUeContext, 102)

	writes := recordDatastoreWrites(t)

	HandleUEContextReleaseComplete(ctxt.Background(), deregRan, releaseCompletePdu(deregRanUe))
	HandleUEContextReleaseComplete(ctxt.Background(), keepRan, releaseCompletePdu(keepRanUe))

	if _, ok := self.UePool.Load(deregSupi); ok {
		t.Error("a deregistered UE was kept in the pool; its stored context will outlive it " +
			"and the next registration for this SUPI will meet stale state")
	}
	if !slices.Equal(*writes, []string{"delete " + deregSupi}) {
		t.Errorf("datastore writes = %v, want the deregistered UE's context deleted", *writes)
	}
	if _, ok := self.UePool.Load(keepSupi); !ok {
		t.Error("a normally released UE with a valid security context was removed; " +
			"HandleSecurityModeReject and RAN-initiated release both depend on it surviving")
	}
}

// A deregistration can name one access while the UE stays registered over the other, and
// a deregistration of both accesses completes its two releases in either order. Either way,
// the release of this access removes the UE only once the other access is out of use: not
// registered, and with no RanUe of its own still waiting for its release to complete.
func TestDeregistrationRemovesTheUeOnlyOnceTheOtherAccessIsOutOfUse(t *testing.T) {
	tests := []struct {
		name        string
		supi        string
		ngapIds     [2]int64
		otherState  fsm.StateType
		otherHasRan bool
		wantKept    bool
		wantWrite   string
		whyIfWrong  string
	}{
		{
			name:       "other access registered but idle: kept",
			supi:       "imsi-208930000000103",
			ngapIds:    [2]int64{103, 104},
			otherState: context.Registered,
			wantKept:   true,
			wantWrite:  "store",
			whyIfWrong: "deregistering 3GPP removed a UE still registered over non-3GPP",
		},
		{
			name:        "other access's release still pending: kept",
			supi:        "imsi-208930000000105",
			ngapIds:     [2]int64{105, 106},
			otherState:  context.Deregistered,
			otherHasRan: true,
			wantKept:    true,
			wantWrite:   "store",
			whyIfWrong: "the first Release Complete of a two-access deregistration tore down " +
				"the other access's RanUe before its own release completed",
		},
		{
			name:       "other access deregistering, nothing pending: removed",
			supi:       "imsi-208930000000107",
			ngapIds:    [2]int64{107, 108},
			otherState: context.DeregistrationInitiated,
			wantKept:   false,
			wantWrite:  "delete",
			whyIfWrong: "a UE with no access left in use was kept; its stored context will outlive it",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			self := context.AMF_Self()
			ran, ranUe, amfUe := newReleasingUe(
				t, tc.supi, context.UeContextReleaseDueToUeInitiatedDeregistration, tc.ngapIds[0])
			amfUe.State[models.ACCESSTYPE_NON_3_GPP_ACCESS].Set(tc.otherState)

			if tc.otherHasRan {
				n3gppRan := context.NewAmfRanDefault()
				n3gppRan.AnType = models.ACCESSTYPE_NON_3_GPP_ACCESS
				n3gppRanUe, err := n3gppRan.NewRanUe(tc.ngapIds[1])
				if err != nil {
					t.Fatalf("creating the non-3GPP RanUe: %v", err)
				}
				n3gppRanUe.Log = logger.NgapLog
				amfUe.AttachRanUe(n3gppRanUe)
				t.Cleanup(func() {
					if leftover := self.RanUeFindByAmfUeNgapIDLocal(n3gppRanUe.AmfUeNgapId); leftover != nil {
						if err := leftover.Remove(); err != nil {
							t.Logf("removing the leftover non-3GPP RanUe: %v", err)
						}
					}
				})
			}
			writes := recordDatastoreWrites(t)

			HandleUEContextReleaseComplete(ctxt.Background(), ran, releaseCompletePdu(ranUe))

			if amfUe.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS) != nil {
				t.Error("the 3GPP access whose release completed was not released")
			}
			_, inPool := self.UePool.Load(tc.supi)
			if inPool != tc.wantKept {
				t.Errorf("UE in pool = %v, want %v: %s", inPool, tc.wantKept, tc.whyIfWrong)
			}
			if tc.otherHasRan && amfUe.GetRanUe(models.ACCESSTYPE_NON_3_GPP_ACCESS) == nil {
				t.Error("the non-3GPP RanUe was torn down before its own release completed")
			}
			if want := []string{tc.wantWrite + " " + tc.supi}; !slices.Equal(*writes, want) {
				t.Errorf("datastore writes = %v, want %v", *writes, want)
			}
		})
	}
}
