// Copyright (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
	"go.uber.org/zap"
)

func disableKafkaForTest(t *testing.T) {
	t.Helper()

	originalConfig := factory.AmfConfig.Configuration
	var originalEnableKafka *bool
	var originalEnableKafkaValue bool
	if originalConfig != nil {
		originalEnableKafka = originalConfig.KafkaInfo.EnableKafka
		if originalEnableKafka != nil {
			originalEnableKafkaValue = *originalEnableKafka
		}
	}

	if factory.AmfConfig.Configuration == nil {
		factory.AmfConfig.Configuration = &factory.Configuration{}
	}
	disabled := false
	factory.AmfConfig.Configuration.KafkaInfo.EnableKafka = &disabled
	t.Cleanup(func() {
		if originalConfig == nil {
			factory.AmfConfig.Configuration = nil
			return
		}

		factory.AmfConfig.Configuration = originalConfig
		if originalEnableKafka == nil {
			factory.AmfConfig.Configuration.KafkaInfo.EnableKafka = nil
			return
		}

		factory.AmfConfig.Configuration.KafkaInfo.EnableKafka = originalEnableKafka
		*factory.AmfConfig.Configuration.KafkaInfo.EnableKafka = originalEnableKafkaValue
	})
}

func TestHandleHandoverNotifyIgnoresMissingIDs(t *testing.T) {
	ran := &context.AmfRan{Log: zap.NewNop().Sugar()}
	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeHandoverNotification},
			Value: ngapType.InitiatingMessageValue{
				Present:              ngapType.InitiatingMessagePresentHandoverNotification,
				HandoverNotification: &ngapType.HandoverNotify{},
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

func TestHandlePDUSessionResourceNotifyIgnoresMissingIEs(t *testing.T) {
	ran := &context.AmfRan{Log: zap.NewNop().Sugar()}
	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodePDUSessionResourceNotify},
			Value: ngapType.InitiatingMessageValue{
				Present:                  ngapType.InitiatingMessagePresentPDUSessionResourceNotify,
				PDUSessionResourceNotify: &ngapType.PDUSessionResourceNotify{},
			},
		},
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("HandlePDUSessionResourceNotify panicked with missing IEs: %v", recovered)
		}
	}()

	HandlePDUSessionResourceNotify(ctxt.Background(), ran, pdu)
}

func TestHandleHandoverFailureIgnoresMissingIEs(t *testing.T) {
	ran := &context.AmfRan{Log: zap.NewNop().Sugar()}
	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentUnsuccessfulOutcome,
		UnsuccessfulOutcome: &ngapType.UnsuccessfulOutcome{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeHandoverResourceAllocation},
			Value: ngapType.UnsuccessfulOutcomeValue{
				Present:                    ngapType.UnsuccessfulOutcomePresentHandoverResourceAllocation,
				HandoverResourceAllocation: &ngapType.HandoverFailure{},
			},
		},
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("HandleHandoverFailure panicked with missing IEs: %v", recovered)
		}
	}()

	HandleHandoverFailure(ctxt.Background(), ran, pdu)
}

func TestHandleUplinkRanStatusTransferIgnoresMissingIEs(t *testing.T) {
	ran := &context.AmfRan{Log: zap.NewNop().Sugar()}
	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeUplinkRANStatusTransfer},
			Value: ngapType.InitiatingMessageValue{
				Present:                 ngapType.InitiatingMessagePresentUplinkRANStatusTransfer,
				UplinkRANStatusTransfer: &ngapType.UplinkRANStatusTransfer{},
			},
		},
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("HandleUplinkRanStatusTransfer panicked with missing IEs: %v", recovered)
		}
	}()

	HandleUplinkRanStatusTransfer(ran, pdu)
}

func TestHandleHandoverRequestAcknowledgeIgnoresMissingAmfUeNgapID(t *testing.T) {
	ran := &context.AmfRan{Log: zap.NewNop().Sugar()}
	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentSuccessfulOutcome,
		SuccessfulOutcome: &ngapType.SuccessfulOutcome{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeHandoverResourceAllocation},
			Value: ngapType.SuccessfulOutcomeValue{
				Present: ngapType.SuccessfulOutcomePresentHandoverResourceAllocation,
				HandoverResourceAllocation: &ngapType.HandoverRequestAcknowledge{
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
				Present: ngapType.SuccessfulOutcomePresentUEContextRelease,
				UEContextRelease: &ngapType.UEContextReleaseComplete{
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

	if amfUe.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS) != newRanUe {
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

func TestHandleUEContextReleaseCompleteStaleHandoverDetachesLink(t *testing.T) {
	self := context.AMF_Self()
	sourceRan := &context.AmfRan{AnType: models.ACCESSTYPE__3_GPP_ACCESS, Log: zap.NewNop().Sugar()}
	targetRan := &context.AmfRan{AnType: models.ACCESSTYPE__3_GPP_ACCESS, Log: zap.NewNop().Sugar()}
	amfUe := self.NewAmfUe("")
	sourceRanUe, err := sourceRan.NewRanUe(30)
	if err != nil {
		t.Fatalf("unexpected error creating source RanUe: %v", err)
	}
	targetRanUe, err := targetRan.NewRanUe(40)
	if err != nil {
		t.Fatalf("unexpected error creating target RanUe: %v", err)
	}
	sourceRanUe.Log = logger.NgapLog
	targetRanUe.Log = logger.NgapLog
	sourceRanUe.ReleaseAction = context.UeContextReleaseHandover

	amfUe.AttachRanUe(sourceRanUe)
	context.AttachSourceUeTargetUe(sourceRanUe, targetRanUe)
	amfUe.AttachRanUe(targetRanUe)

	pdu := &ngapType.NGAPPDU{
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
			t.Fatalf("HandleUEContextReleaseComplete panicked for stale handover RanUe: %v", recovered)
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

	if amfUe.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS) != targetRanUe {
		t.Fatal("expected target RanUe to remain the current association")
	}
	if sourceRanUe.AmfUe != nil {
		t.Fatal("expected stale source RanUe association to be cleared")
	}
	if sourceRanUe.TargetUe != nil {
		t.Fatal("expected stale source-target link to be cleared")
	}
	if targetRanUe.SourceUe != nil {
		t.Fatal("expected target-source link to be cleared")
	}
	if self.RanUeFindByAmfUeNgapIDLocal(sourceRanUe.AmfUeNgapId) != nil {
		t.Fatal("expected stale source RanUe to be removed from the pool")
	}
	if self.RanUeFindByAmfUeNgapIDLocal(targetRanUe.AmfUeNgapId) != targetRanUe {
		t.Fatal("expected target RanUe to remain in the pool")
	}
}

func TestHandleUEContextReleaseCompleteHandoverPromotesTargetRanUe(t *testing.T) {
	self := context.AMF_Self()
	disableKafkaForTest(t)
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
				Present: ngapType.SuccessfulOutcomePresentUEContextRelease,
				UEContextRelease: &ngapType.UEContextReleaseComplete{
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

	if amfUe.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS) != targetRanUe {
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
