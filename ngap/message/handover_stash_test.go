// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package message

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/ngapconv"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
)

// TestADuplicateHandoverRequiredLeavesTheLiveHandoversStashIntact is the record-integrity
// property behind the AMFRANHandoverRequest xIRI.
//
// TS 33.128 puts that record on the HANDOVER REQUEST ACKNOWLEDGE, and the two values it
// needs — the cause and the source-to-target container — exist only on the way in, so they
// have to be stashed. SendHandoverRequest has five guards that return without sending, and
// one of them is "Handover Required Duplicated". Stashed before them, a duplicate HANDOVER
// REQUIRED overwrites the live handover's stash and sends nothing, and the first handover's
// acknowledgement is then reported with the rejected request's cause and container — a
// record describing a handover that never happened, attributed to one that did, with nothing
// in either stream to show it.
//
// Committed inside SendHandoverRequest, past the guards, the stash and the request carry the
// same values by construction: a request that is not sent replaces nothing.
func TestADuplicateHandoverRequiredLeavesTheLiveHandoversStashIntact(t *testing.T) {
	sourceUe := newRanUeForAllowedNSSAITest(models.ACCESSTYPE__3_GPP_ACCESS)
	// The fixture builds the RanUe as a literal, so it has no logger; every path below logs.
	sourceUe.Log = logger.NgapLog
	sourceUe.Ran.Log = logger.NgapLog

	live := &ngapType.Cause{
		Present:      ngapType.CausePresentRadioNetwork,
		RadioNetwork: &ngapType.CauseRadioNetwork{Value: ngapType.CauseRadioNetworkPresentHandoverDesirableForRadioReason},
	}
	liveContainer := []byte{0x01, 0x02, 0x03, 0x04}
	liveType := ngapType.HandoverType{Value: ngapType.HandoverTypePresentIntra5gs}

	// The live handover: sent, so its values are what the acknowledgement must report. Its
	// PDU session item has to encode, or the request is never sent and keeps no stash.
	SendHandoverRequest(sourceUe, sourceUe.Ran, *live, liveType,
		ngapType.PDUSessionResourceSetupListHOReq{
			List: []ngapType.PDUSessionResourceSetupItemHOReq{{
				PDUSessionID:            ngapType.PDUSessionID{Value: 10},
				SNSSAI:                  ngapconv.SNssaiToNgap(models.Snssai{Sst: 1, Sd: new("010203")}),
				HandoverRequestTransfer: []byte{0x00},
			}},
		},
		ngapType.SourceToTargetTransparentContainer{Value: liveContainer}, false)

	if sourceUe.HandOverCause == nil {
		t.Fatal("SendHandoverRequest stashed nothing for a request it sent: the values the " +
			"AMFRANHandoverRequest xIRI needs are committed here, past the guards, precisely so " +
			"a request that is not sent cannot replace a live handover's — a caller that stashes " +
			"before the call has already overwritten it by the time this function decides")
	}
	if sourceUe.TargetUe == nil {
		t.Fatal("the live handover did not reach the target-UE attachment, so the duplicate " +
			"guard below is not the one that will fire")
	}

	// The duplicate: `sourceUe.TargetUe != nil` now, so this returns on the
	// "Handover Required Duplicated" guard without sending anything.
	rejected := &ngapType.Cause{
		Present: ngapType.CausePresentMisc,
		Misc:    &ngapType.CauseMisc{Value: ngapType.CauseMiscPresentUnspecified},
	}
	SendHandoverRequest(sourceUe, sourceUe.Ran, *rejected,
		ngapType.HandoverType{Value: ngapType.HandoverTypePresentFivegsToEps},
		ngapType.PDUSessionResourceSetupListHOReq{
			List: []ngapType.PDUSessionResourceSetupItemHOReq{{}},
		},
		ngapType.SourceToTargetTransparentContainer{Value: []byte{0xAA, 0xBB}}, false)

	if sourceUe.HandOverCause.Present != live.Present {
		t.Errorf("the stashed cause is the rejected request's (present %d), want the live "+
			"handover's (%d): the first handover's acknowledgement now reports a cause from a "+
			"request that was never sent", sourceUe.HandOverCause.Present, live.Present)
	}
	if !bytes.Equal(sourceUe.HandOverSourceToTarget, liveContainer) {
		t.Errorf("the stashed container is %v, want the live handover's %v",
			sourceUe.HandOverSourceToTarget, liveContainer)
	}

	// **The third value.** Both handover records carry the type, so a duplicate that overwrote
	// it would make them state a handover the subject did not perform: intra-5GS reported as
	// 5GS-to-EPS. A mandatory field, populated, plausible and false, which nothing downstream
	// distinguishes from a true one.
	if sourceUe.HandOverType.Value != liveType.Value {
		t.Errorf("the stashed handover type is %d, want the live handover's %d: the duplicate's "+
			"type replaced it, so both records for the handover that did happen name one that "+
			"did not", sourceUe.HandOverType.Value, liveType.Value)
	}
}

// TestAHandoverRequestThatCannotBeBuiltLeavesNoStash: past the guards the stash is committed,
// and a HANDOVER REQUEST that then fails to build is never sent, so no acknowledge or failure
// comes back to clear it. The copied source-to-target container would stay on the source
// RanUe for as long as the RanUe does.
func TestAHandoverRequestThatCannotBeBuiltLeavesNoStash(t *testing.T) {
	// Any access will do; the request only has to fail to build.
	sourceUe := newRanUeForAllowedNSSAITest(models.ACCESSTYPE_NON_3_GPP_ACCESS)
	sourceUe.Log = logger.NgapLog
	sourceUe.Ran.Log = logger.NgapLog
	// More distinct slices than NGAP's Allowed NSSAI can carry: BuildHandoverRequest refuses.
	allowed := make([]models.AllowedSnssai, 0, maxAllowedNSSAIItems+1)
	for index := range maxAllowedNSSAIItems + 1 {
		allowed = append(allowed, models.AllowedSnssai{
			AllowedSnssai: models.Snssai{Sst: int32(index + 1), Sd: new(fmt.Sprintf("%06x", index+1))},
		})
	}
	sourceUe.AmfUe.AllowedNssai[models.ACCESSTYPE_NON_3_GPP_ACCESS] = allowed

	SendHandoverRequest(sourceUe, sourceUe.Ran, ngapType.Cause{
		Present:      ngapType.CausePresentRadioNetwork,
		RadioNetwork: &ngapType.CauseRadioNetwork{Value: ngapType.CauseRadioNetworkPresentUnspecified},
	}, ngapType.HandoverType{Value: ngapType.HandoverTypePresentIntra5gs},
		ngapType.PDUSessionResourceSetupListHOReq{List: []ngapType.PDUSessionResourceSetupItemHOReq{{}}},
		ngapType.SourceToTargetTransparentContainer{Value: []byte{0x01, 0x02}}, false)

	if sourceUe.TargetUe == nil {
		t.Fatal("the request did not reach the build, so this test is not exercising its failure")
	}
	if sourceUe.HandOverSourceToTarget != nil || sourceUe.HandOverCause != nil {
		t.Errorf("a request that was never sent left its stash (container % x, cause %v); "+
			"nothing will come back to clear it", sourceUe.HandOverSourceToTarget, sourceUe.HandOverCause)
	}
}
