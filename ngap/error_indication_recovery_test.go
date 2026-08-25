// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
)

// pendingMessage is the shape a DDN-triggered transfer leaves behind: N2 information the
// AMF is holding on the SMF's behalf.
func pendingMessage() *context.N1N2Message {
	return &context.N1N2Message{
		Status: models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE,
		N2Info: []byte{0x01, 0x02, 0x03},
	}
}

// staleUeErrorIndication is what a gNB sends when the AMF addresses it about a UE the gNB
// has already let go.
func staleUeErrorIndication() *ngapType.Cause {
	cause := &ngapType.Cause{Present: ngapType.CausePresentRadioNetwork}
	cause.RadioNetwork = new(ngapType.CauseRadioNetwork)
	cause.RadioNetwork.Value = ngapType.CauseRadioNetworkPresentUnknownLocalUENGAPID

	return cause
}

func ranWithPagedUe(t *testing.T, ranUeNgapID int64) (*context.AmfRan, *context.RanUe, *context.AmfUe) {
	t.Helper()

	ran := context.NewAmfRanDefault()
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS

	ranUe, err := ran.NewRanUe(ranUeNgapID)
	if err != nil {
		t.Fatalf("create RanUe: %v", err)
	}

	ranUe.Log = logger.NgapLog

	amfUe := context.AMF_Self().NewAmfUe("")
	amfUe.AttachRanUe(ranUe)
	amfUe.N1N2Message = pendingMessage()

	// Paging is addressed by 5G-S-TMSI and bounded by the registration area, so a UE
	// without both is not pageable and would exercise the build-failure branch instead.
	amfUe.Guti = "20893cafe0000000001"
	amfUe.RegistrationArea[models.ACCESSTYPE__3_GPP_ACCESS] = []models.Tai{
		{PlmnId: models.PlmnId{Mcc: "208", Mnc: "93"}, Tac: "000001"},
	}

	return ran, ranUe, amfUe
}

// The defect: the AMF chose N2 over paging because the UE looked connected, the write
// succeeded, and it answered N1N2_TRANSFER_INITIATED -- so nothing upstream retries. This
// indication is the only notice it gets that the message went nowhere.
func TestErrorIndicationForAStaleUeIdReleasesTheContextAndPages(t *testing.T) {
	ran, ranUe, amfUe := ranWithPagedUe(t, 11)
	amfUeNgapID := ranUe.AmfUeNgapId

	recoverPendingMessageAfterStaleUeNgapID(ran, amfUeNgapIDIe(amfUeNgapID), nil,
		staleUeErrorIndication())

	if context.AMF_Self().RanUeFindByAmfUeNgapIDLocal(amfUeNgapID) != nil {
		t.Error("the stale UE context is still held; the next transfer would repeat the failure")
	}

	if amfUe.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] != nil {
		t.Error("the UE still points at the released RanUe, so it still looks connected")
	}

	if got := amfUe.GetOnGoing(models.ACCESSTYPE__3_GPP_ACCESS).Procedure; got != context.OnGoingProcedurePaging {
		t.Errorf("ongoing procedure = %q, want %q", got, context.OnGoingProcedurePaging)
	}

	// The pending message must survive: it is what the paging exists to deliver.
	if amfUe.N1N2Message == nil {
		t.Error("the pending message was discarded, so the page has nothing to deliver")
	}
}

// Every other error indication must be inert. Releasing a UE context on, say, a protocol
// cause would turn a transient complaint into a dropped session.
func TestErrorIndicationWithAnotherCauseChangesNothing(t *testing.T) {
	ran, ranUe, amfUe := ranWithPagedUe(t, 12)
	amfUeNgapID := ranUe.AmfUeNgapId

	other := &ngapType.Cause{Present: ngapType.CausePresentRadioNetwork}
	other.RadioNetwork = new(ngapType.CauseRadioNetwork)
	other.RadioNetwork.Value = ngapType.CauseRadioNetworkPresentRadioConnectionWithUeLost

	recoverPendingMessageAfterStaleUeNgapID(ran, amfUeNgapIDIe(amfUeNgapID), nil, other)

	if context.AMF_Self().RanUeFindByAmfUeNgapIDLocal(amfUeNgapID) == nil {
		t.Error("a UE context was released on a cause that does not say the id is unknown")
	}

	if got := amfUe.GetOnGoing(models.ACCESSTYPE__3_GPP_ACCESS).Procedure; got == context.OnGoingProcedurePaging {
		t.Error("paging was started on a cause that does not say the id is unknown")
	}
}

// The AMF-assigned id is unique across the AMF, not within one gNB, so a peer naming
// another gNB's UE must not be able to have that subscriber's context destroyed.
func TestErrorIndicationFromAnotherGnbIsIgnored(t *testing.T) {
	_, ranUe, _ := ranWithPagedUe(t, 13)
	amfUeNgapID := ranUe.AmfUeNgapId

	unrelated := context.NewAmfRanDefault()
	unrelated.AnType = models.ACCESSTYPE__3_GPP_ACCESS

	recoverPendingMessageAfterStaleUeNgapID(unrelated, amfUeNgapIDIe(amfUeNgapID), nil,
		staleUeErrorIndication())

	if context.AMF_Self().RanUeFindByAmfUeNgapIDLocal(amfUeNgapID) == nil {
		t.Error("an unrelated gNB had another gNB's UE context released on its word")
	}
}

// Nothing pending means nothing to deliver: release the stale context, but do not page.
func TestErrorIndicationWithNothingPendingDoesNotPage(t *testing.T) {
	ran, ranUe, amfUe := ranWithPagedUe(t, 14)
	amfUe.N1N2Message = nil
	amfUeNgapID := ranUe.AmfUeNgapId

	recoverPendingMessageAfterStaleUeNgapID(ran, amfUeNgapIDIe(amfUeNgapID), nil,
		staleUeErrorIndication())

	if context.AMF_Self().RanUeFindByAmfUeNgapIDLocal(amfUeNgapID) != nil {
		t.Error("the stale UE context should still be released even with nothing pending")
	}

	if got := amfUe.GetOnGoing(models.ACCESSTYPE__3_GPP_ACCESS).Procedure; got == context.OnGoingProcedurePaging {
		t.Error("paging was started with no pending message to deliver")
	}
}

func amfUeNgapIDIe(value int64) *ngapType.AMFUENGAPID {
	return &ngapType.AMFUENGAPID{Value: value}
}

// The defect was a handler that decoded, logged, and returned. Driving the real message
// through it is what proves the recovery is actually wired in -- asserting on the helper
// alone passes just as happily when nothing calls it.
func TestHandleErrorIndicationRecoversThroughTheRealMessage(t *testing.T) {
	ran, ranUe, amfUe := ranWithPagedUe(t, 15)
	amfUeNgapID := ranUe.AmfUeNgapId

	pdu := &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeErrorIndication},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentErrorIndication,
				ErrorIndication: &ngapType.ErrorIndication{
					ProtocolIEs: ngapType.ProtocolIEContainerErrorIndicationIEs{
						List: []ngapType.ErrorIndicationIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Value: ngapType.ErrorIndicationIEsValue{
									Present:     ngapType.ErrorIndicationIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: amfUeNgapID},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Value: ngapType.ErrorIndicationIEsValue{
									Present:     ngapType.ErrorIndicationIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUe.RanUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDCause},
								Value: ngapType.ErrorIndicationIEsValue{
									Present: ngapType.ErrorIndicationIEsPresentCause,
									Cause:   staleUeErrorIndication(),
								},
							},
						},
					},
				},
			},
		},
	}

	HandleErrorIndication(ran, pdu)

	if context.AMF_Self().RanUeFindByAmfUeNgapIDLocal(amfUeNgapID) != nil {
		t.Error("HandleErrorIndication left the stale UE context in place")
	}

	if got := amfUe.GetOnGoing(models.ACCESSTYPE__3_GPP_ACCESS).Procedure; got != context.OnGoingProcedurePaging {
		t.Errorf("ongoing procedure after HandleErrorIndication = %q, want %q",
			got, context.OnGoingProcedurePaging)
	}
}
