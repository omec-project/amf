// Copyright (C) 2026 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package ngap

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/ngap/ngapType"
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
