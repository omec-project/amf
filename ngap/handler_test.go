// Copyright (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
	"go.uber.org/zap"
)

func TestHandleHandoverNotifyIgnoresMissingIDs(t *testing.T) {
	ran := &context.AmfRan{Log: zap.NewNop().Sugar()}
	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeHandoverNotification},
			Value: ngapType.InitiatingMessageValue{
				Present:        ngapType.InitiatingMessagePresentHandoverNotify,
				HandoverNotify: &ngapType.HandoverNotify{},
			},
		},
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("HandleHandoverNotify panicked with missing IDs: %v", recovered)
		}
	}()

	HandleHandoverNotify(ctxt.Background(), ran, pdu)
}

func TestHandleHandoverRequestAcknowledgeIgnoresMissingAmfUeNgapID(t *testing.T) {
	ran := &context.AmfRan{Log: zap.NewNop().Sugar()}
	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentSuccessfulOutcome,
		SuccessfulOutcome: &ngapType.SuccessfulOutcome{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeHandoverResourceAllocation},
			Value: ngapType.SuccessfulOutcomeValue{
				Present: ngapType.SuccessfulOutcomePresentHandoverRequestAcknowledge,
				HandoverRequestAcknowledge: &ngapType.HandoverRequestAcknowledge{
					ProtocolIEs: ngapType.ProtocolIEContainerHandoverRequestAcknowledgeIEs{
						List: []ngapType.HandoverRequestAcknowledgeIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDTargetToSourceTransparentContainer},
								Value: ngapType.HandoverRequestAcknowledgeIEsValue{
									Present:                            ngapType.HandoverRequestAcknowledgeIEsPresentTargetToSourceTransparentContainer,
									TargetToSourceTransparentContainer: &ngapType.TargetToSourceTransparentContainer{},
								},
							},
						},
					},
				},
			},
		},
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("HandleHandoverRequestAcknowledge panicked with missing AMFUENGAPID: %v", recovered)
		}
	}()

	HandleHandoverRequestAcknowledge(ctxt.Background(), ran, pdu)
}

func TestFetchRanUeContextReturnsNilForMissingIDs(t *testing.T) {
	self := context.AMF_Self()
	previousReady := self.Rcvd
	self.Rcvd = true
	defer func() {
		self.Rcvd = previousReady
	}()

	ran := &context.AmfRan{Log: zap.NewNop().Sugar()}
	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeInitialUEMessage},
			Value: ngapType.InitiatingMessageValue{
				Present:          ngapType.InitiatingMessagePresentInitialUEMessage,
				InitialUEMessage: &ngapType.InitialUEMessage{},
			},
		},
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("FetchRanUeContext panicked on missing IDs: %v", recovered)
		}
	}()

	ranUe, amfID := FetchRanUeContext(ran, pdu)
	if ranUe != nil || amfID != nil {
		t.Fatal("expected no UE context to be resolved from a malformed InitialUEMessage")
	}
}

func TestPrintAndGetCauseIgnoresNilCauseMembers(t *testing.T) {
	ran := &context.AmfRan{Log: zap.NewNop().Sugar()}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("printAndGetCause panicked on malformed cause: %v", recovered)
		}
	}()

	printAndGetCause(ran, nil)
	printAndGetCause(ran, &ngapType.Cause{Present: ngapType.CausePresentProtocol})
}

func TestHandleUEContextReleaseCompleteRemovesStaleRanUe(t *testing.T) {
	self := context.AMF_Self()
	oldRan := &context.AmfRan{AnType: models.ACCESSTYPE__3_GPP_ACCESS, Log: zap.NewNop().Sugar()}
	newRan := &context.AmfRan{AnType: models.ACCESSTYPE__3_GPP_ACCESS, Log: zap.NewNop().Sugar()}
	amfUe := self.NewAmfUe("")
	oldRanUe, err := oldRan.NewRanUe(1)
	if err != nil {
		t.Fatalf("unexpected error creating old RanUe: %v", err)
	}
	newRanUe, err := newRan.NewRanUe(2)
	if err != nil {
		t.Fatalf("unexpected error creating new RanUe: %v", err)
	}
	oldRanUe.Log = logger.NgapLog
	newRanUe.Log = logger.NgapLog

	amfUe.AttachRanUe(oldRanUe)
	amfUe.AttachRanUe(newRanUe)

	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentSuccessfulOutcome,
		SuccessfulOutcome: &ngapType.SuccessfulOutcome{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeUEContextRelease},
			Value: ngapType.SuccessfulOutcomeValue{
				Present: ngapType.SuccessfulOutcomePresentUEContextReleaseComplete,
				UEContextReleaseComplete: &ngapType.UEContextReleaseComplete{
					ProtocolIEs: ngapType.ProtocolIEContainerUEContextReleaseCompleteIEs{
						List: []ngapType.UEContextReleaseCompleteIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Value: ngapType.UEContextReleaseCompleteIEsValue{
									Present:     ngapType.UEContextReleaseCompleteIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: oldRanUe.AmfUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Value: ngapType.UEContextReleaseCompleteIEsValue{
									Present:     ngapType.UEContextReleaseCompleteIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: oldRanUe.RanUeNgapId},
								},
							},
						},
					},
				},
			},
		},
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("HandleUEContextReleaseComplete panicked for stale RanUe: %v", recovered)
		}
		if leftover := self.RanUeFindByAmfUeNgapIDLocal(newRanUe.AmfUeNgapId); leftover != nil {
			if err := leftover.Remove(); err != nil {
				t.Fatalf("cleanup current RanUe failed: %v", err)
			}
		}
		if leftover := self.RanUeFindByAmfUeNgapIDLocal(oldRanUe.AmfUeNgapId); leftover != nil {
			if err := leftover.Remove(); err != nil {
				t.Fatalf("cleanup stale RanUe failed: %v", err)
			}
		}
	}()

	HandleUEContextReleaseComplete(ctxt.Background(), oldRan, pdu)

	if amfUe.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] != newRanUe {
		t.Fatal("expected current RanUe association to remain attached")
	}
	if oldRanUe.AmfUe != nil {
		t.Fatal("expected stale RanUe association to be cleared")
	}
	if self.RanUeFindByAmfUeNgapIDLocal(oldRanUe.AmfUeNgapId) != nil {
		t.Fatal("expected stale RanUe to be removed from the pool")
	}
	if self.RanUeFindByAmfUeNgapIDLocal(newRanUe.AmfUeNgapId) != newRanUe {
		t.Fatal("expected current RanUe to remain in the pool")
	}
}

func TestHandleUEContextReleaseCompleteHandoverPromotesTargetRanUe(t *testing.T) {
	self := context.AMF_Self()
	sourceRan := &context.AmfRan{AnType: models.ACCESSTYPE__3_GPP_ACCESS, Log: zap.NewNop().Sugar()}
	targetRan := &context.AmfRan{AnType: models.ACCESSTYPE__3_GPP_ACCESS, Log: zap.NewNop().Sugar()}
	amfUe := self.NewAmfUe("")
	sourceRanUe, err := sourceRan.NewRanUe(10)
	if err != nil {
		t.Fatalf("unexpected error creating source RanUe: %v", err)
	}
	targetRanUe, err := targetRan.NewRanUe(20)
	if err != nil {
		t.Fatalf("unexpected error creating target RanUe: %v", err)
	}
	sourceRanUe.Log = logger.NgapLog
	targetRanUe.Log = logger.NgapLog
	sourceRanUe.ReleaseAction = context.UeContextReleaseHandover

	amfUe.AttachRanUe(sourceRanUe)
	context.AttachSourceUeTargetUe(sourceRanUe, targetRanUe)

	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentSuccessfulOutcome,
		SuccessfulOutcome: &ngapType.SuccessfulOutcome{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeUEContextRelease},
			Value: ngapType.SuccessfulOutcomeValue{
				Present: ngapType.SuccessfulOutcomePresentUEContextReleaseComplete,
				UEContextReleaseComplete: &ngapType.UEContextReleaseComplete{
					ProtocolIEs: ngapType.ProtocolIEContainerUEContextReleaseCompleteIEs{
						List: []ngapType.UEContextReleaseCompleteIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Value: ngapType.UEContextReleaseCompleteIEsValue{
									Present:     ngapType.UEContextReleaseCompleteIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: sourceRanUe.AmfUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Value: ngapType.UEContextReleaseCompleteIEsValue{
									Present:     ngapType.UEContextReleaseCompleteIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: sourceRanUe.RanUeNgapId},
								},
							},
						},
					},
				},
			},
		},
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("HandleUEContextReleaseComplete panicked for handover release: %v", recovered)
		}
		if leftover := self.RanUeFindByAmfUeNgapIDLocal(targetRanUe.AmfUeNgapId); leftover != nil {
			if err := leftover.Remove(); err != nil {
				t.Fatalf("cleanup target RanUe failed: %v", err)
			}
		}
		if leftover := self.RanUeFindByAmfUeNgapIDLocal(sourceRanUe.AmfUeNgapId); leftover != nil {
			if err := leftover.Remove(); err != nil {
				t.Fatalf("cleanup source RanUe failed: %v", err)
			}
		}
	}()

	HandleUEContextReleaseComplete(ctxt.Background(), sourceRan, pdu)

	if amfUe.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] != targetRanUe {
		t.Fatal("expected target RanUe to become the current association after handover release")
	}
	if sourceRanUe.AmfUe != nil {
		t.Fatal("expected source RanUe to be detached after handover release")
	}
	if sourceRanUe.TargetUe != nil {
		t.Fatal("expected source-target link to be cleared after handover release")
	}
	if targetRanUe.SourceUe != nil {
		t.Fatal("expected target-source link to be cleared after handover release")
	}
	if self.RanUeFindByAmfUeNgapIDLocal(sourceRanUe.AmfUeNgapId) != nil {
		t.Fatal("expected source RanUe to be removed from the pool")
	}
	if self.RanUeFindByAmfUeNgapIDLocal(targetRanUe.AmfUeNgapId) != targetRanUe {
		t.Fatal("expected target RanUe to remain in the pool")
	}
}
