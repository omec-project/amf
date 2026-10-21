// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	ngaputil "github.com/omec-project/amf/ngap/util"
	libngap "github.com/omec-project/ngap/v2"
	"github.com/omec-project/ngap/v2/aper"
	"github.com/omec-project/ngap/v2/ngapType"
)

// handoverRequiredTo builds the Handover Required a source RAN sends for ranUe towards target.
func handoverRequiredTo(ranUe *context.RanUe, target ngapType.TargetID) *ngapType.NGAPPDU {
	return &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeHandoverPreparation},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentHandoverPreparation,
				HandoverPreparation: &ngapType.HandoverRequired{
					ProtocolIEs: ngapType.ProtocolIEContainerHandoverRequiredIEs{
						List: []ngapType.HandoverRequiredIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Value: ngapType.HandoverRequiredIEsValue{
									Present:     ngapType.HandoverRequiredIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: ranUe.AmfUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Value: ngapType.HandoverRequiredIEsValue{
									Present:     ngapType.HandoverRequiredIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUe.RanUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDHandoverType},
								Value: ngapType.HandoverRequiredIEsValue{
									Present:      ngapType.HandoverRequiredIEsPresentHandoverType,
									HandoverType: &ngapType.HandoverType{Value: ngapType.HandoverTypePresentIntra5gs},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDCause},
								Value: ngapType.HandoverRequiredIEsValue{
									Present: ngapType.HandoverRequiredIEsPresentCause,
									Cause: &ngapType.Cause{
										Present: ngapType.CausePresentRadioNetwork,
										RadioNetwork: &ngapType.CauseRadioNetwork{
											Value: ngapType.CauseRadioNetworkPresentHandoverDesirableForRadioReason,
										},
									},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDTargetID},
								Value: ngapType.HandoverRequiredIEsValue{
									Present:  ngapType.HandoverRequiredIEsPresentTargetID,
									TargetID: &target,
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDPDUSessionResourceListHORqd},
								Value: ngapType.HandoverRequiredIEsValue{
									Present: ngapType.HandoverRequiredIEsPresentPDUSessionResourceListHORqd,
									PDUSessionResourceListHORqd: &ngapType.PDUSessionResourceListHORqd{
										List: []ngapType.PDUSessionResourceItemHORqd{{
											PDUSessionID:             ngapType.PDUSessionID{Value: 1},
											HandoverRequiredTransfer: aper.OctetString{0x00},
										}},
									},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDSourceToTargetTransparentContainer},
								Value: ngapType.HandoverRequiredIEsValue{
									Present: ngapType.HandoverRequiredIEsPresentSourceToTargetTransparentContainer,
									SourceToTargetTransparentContainer: &ngapType.SourceToTargetTransparentContainer{
										Value: aper.OctetString{0x00},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// targetGNB is a TargetID naming a gNB the AMF has no NG connection to, as a target served by
// another AMF would be.
func targetGNB() ngapType.TargetID {
	plmn := ngapType.PLMNIdentity{Value: aper.OctetString{0x02, 0xf8, 0x39}}
	return ngapType.TargetID{
		Present: ngapType.TargetIDPresentTargetRANNodeID,
		TargetRANNodeID: &ngapType.TargetRANNodeID{
			GlobalRANNodeID: ngapType.GlobalRANNodeID{
				Present: ngapType.GlobalRANNodeIDPresentGlobalGNBID,
				GlobalGNBID: &ngapType.GlobalGNBID{
					PLMNIdentity: plmn,
					GNBID: ngapType.GNBID{
						Present: ngapType.GNBIDPresentGNBID,
						GNBID:   &aper.BitString{Bytes: []byte{0x00, 0x00, 0x99}, BitLength: 24},
					},
				},
			},
			SelectedTAI: ngapType.TAI{PLMNIdentity: plmn, TAC: ngapType.TAC{Value: aper.OctetString{0x00, 0x00, 0x01}}},
		},
	}
}

// requirePreparationFailure checks that the source RAN was sent a Handover Preparation Failure
// carrying the Unknown Target ID cause.
func requirePreparationFailure(t *testing.T, conn *ngaputil.TestConn) {
	t.Helper()

	sent := conn.Snapshot()
	if len(sent) == 0 {
		t.Fatal("the source RAN received nothing, so it would wait for a Handover Command that never comes")
	}
	pdu, err := libngap.Decoder(sent)
	if err != nil {
		t.Fatalf("decoding what the source RAN received: %v", err)
	}
	if pdu.Present != ngapType.NGAPPDUPresentUnsuccessfulOutcome ||
		pdu.UnsuccessfulOutcome.ProcedureCode.Value != ngapType.ProcedureCodeHandoverPreparation {
		t.Fatalf("the source RAN received an NGAP PDU of type %d, want a Handover Preparation Failure", pdu.Present)
	}
	failure := pdu.UnsuccessfulOutcome.Value.HandoverPreparation
	if failure == nil {
		t.Fatal("the unsuccessful outcome carries no Handover Preparation Failure")
	}
	for _, ie := range failure.ProtocolIEs.List {
		if ie.Id.Value != ngapType.ProtocolIEIDCause {
			continue
		}
		cause := ie.Value.Cause
		if cause == nil || cause.Present != ngapType.CausePresentRadioNetwork ||
			cause.RadioNetwork.Value != ngapType.CauseRadioNetworkPresentUnknownTargetID {
			t.Errorf("cause = %+v, want radio network Unknown Target ID", cause)
		}
		return
	}
	t.Error("the Handover Preparation Failure carries no Cause")
}

// A Handover Required towards a gNB that is not connected to this AMF would need an inter-AMF
// handover, which is not implemented. The source RAN must be told the preparation failed rather
// than left waiting, and the UE must not stay marked as mid-handover.
func TestHandoverRequiredToAnUnknownTargetRanFails(t *testing.T) {
	ran, ranUe, amfUe := newReleasingUe(t, "imsi-208930100009980", context.UeContextN2NormalRelease, 980)
	conn := &ngaputil.TestConn{}
	ran.Conn = conn

	HandleHandoverRequired(ctxt.Background(), ran, handoverRequiredTo(ranUe, targetGNB()))

	requirePreparationFailure(t, conn)
	if got := amfUe.GetOnGoing(ran.AnType).Procedure; got != context.OnGoingProcedureNothing {
		t.Errorf("ongoing procedure = %v, want nothing", got)
	}
}

// A target eNB (an EPS handover over N26) is not implemented either, and is answered the same way.
func TestHandoverRequiredToATargetENBFails(t *testing.T) {
	ran, ranUe, amfUe := newReleasingUe(t, "imsi-208930100009981", context.UeContextN2NormalRelease, 981)
	conn := &ngaputil.TestConn{}
	ran.Conn = conn

	target := ngapType.TargetID{
		Present:     ngapType.TargetIDPresentTargeteNBID,
		TargeteNBID: &ngapType.TargeteNBID{},
	}
	HandleHandoverRequired(ctxt.Background(), ran, handoverRequiredTo(ranUe, target))

	requirePreparationFailure(t, conn)
	if got := amfUe.GetOnGoing(ran.AnType).Procedure; got != context.OnGoingProcedureNothing {
		t.Errorf("ongoing procedure = %v, want nothing", got)
	}
}
