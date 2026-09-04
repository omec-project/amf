// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"go.uber.org/zap"
)

// The fix has two halves, and the ngap test covers only one: that
// HandleUEContextReleaseComplete deletes the stored context when it sees the new action.
// Nothing asserted that the deregistration path *selects* that action — reverting
// HandleDeregistrationRequest to UeContextReleaseUeContext restores the defect in full and
// leaves every other test passing.
//
// SendUEContextReleaseCommand records the action on the RanUe before it sends, so the
// decision is observable without intercepting anything.
func TestUeOriginatingDeregistrationSelectsItsOwnReleaseAction(t *testing.T) {
	ue := &context.AmfUe{
		GmmLog: zap.NewNop().Sugar(),
		TxLog:  zap.NewNop().Sugar(),
		NASLog: zap.NewNop().Sugar(),
		RanUe:  make(map[models.AccessType]*context.RanUe),
		State:  make(map[models.AccessType]*fsm.State),
		// SendUEContextReleaseCommand records the NGAP cause through SetReleaseCause,
		// which writes into this map.
		ReleaseCause: make(map[models.AccessType]*context.CauseAll),
	}
	ue.State[models.ACCESSTYPE__3_GPP_ACCESS] = fsm.NewState(context.Registered)
	ue.State[models.ACCESSTYPE_NON_3_GPP_ACCESS] = fsm.NewState(context.Deregistered)

	// A Ran, because the Deregistration Accept that precedes the release reads
	// ue.Ran.AnType while building its Downlink NAS Transport.
	ran := context.NewAmfRanDefault()
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	ranUe := &context.RanUe{AmfUe: ue, Ran: ran, Log: zap.NewNop().Sugar()}
	ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] = ranUe

	request := &nasMessage.DeregistrationRequestUEOriginatingDeregistration{}
	request.SetAccessType(nasMessage.AccessType3GPP)

	// The FSM transition after the release is not what this asserts, so its error is
	// reported rather than failed on: the release command has already been sent by then.
	if err := HandleDeregistrationRequest(
		ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS, request); err != nil {
		t.Logf("deregistration returned %v after sending the release command", err)
	}

	if ranUe.ReleaseAction != context.UeContextReleaseDueToUeInitiatedDeregistration {
		t.Fatalf("release action = %v, want UeContextReleaseDueToUeInitiatedDeregistration (%v): "+
			"a UE-originating deregistration that releases with the shared action leaves its "+
			"stored context behind",
			ranUe.ReleaseAction, context.UeContextReleaseDueToUeInitiatedDeregistration)
	}
}
