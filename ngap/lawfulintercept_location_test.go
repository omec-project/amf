// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	ngaputil "github.com/omec-project/amf/ngap/util"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
)

// observeLocationUpdates replaces the location hook for the test and counts its calls per UE.
func observeLocationUpdates(t *testing.T) map[*context.AmfUe]int {
	t.Helper()
	seen := map[*context.AmfUe]int{}
	original := reportLocationUpdate
	reportLocationUpdate = func(ue *context.AmfUe) { seen[ue]++ }
	t.Cleanup(func() { reportLocationUpdate = original })
	return seen
}

// TS 33.128 clause 6.2.2.2.4 names three NG-RAN messages that generate an AMFLocationUpdate:
// the N2 Path Switch Request, the N2 Handover Notify, and the PDU Session Resource Modify
// Indication sent for dual connectivity. Each handler reports the UE's location once.
func TestPathSwitchRequestReportsTheLocation(t *testing.T) {
	seen := observeLocationUpdates(t)
	_, ranUe, amfUe := newReleasingUe(t, "imsi-208930100009980", context.UeContextN2NormalRelease, 980)

	target := context.NewAmfRanDefault()
	target.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	target.Conn = &ngaputil.TestConn{}
	HandlePathSwitchRequest(ctxt.Background(), target, pathSwitchRequestFor(ranUe, 981, 1))

	if seen[amfUe] != 1 {
		t.Errorf("a Path Switch Request reported the location %d times, want 1", seen[amfUe])
	}
}

func TestHandoverNotifyReportsTheLocation(t *testing.T) {
	seen := observeLocationUpdates(t)
	ran, ranUe, amfUe := newReleasingUe(t, "imsi-208930100009982", context.UeContextN2NormalRelease, 982)
	ran.Conn = &ngaputil.TestConn{}

	HandleHandoverNotify(ctxt.Background(), ran, &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeHandoverNotification},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentHandoverNotification,
				HandoverNotification: &ngapType.HandoverNotify{ProtocolIEs: ngapType.ProtocolIEContainerHandoverNotifyIEs{
					List: []ngapType.HandoverNotifyIEs{
						{
							Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
							Value: ngapType.HandoverNotifyIEsValue{
								Present:     ngapType.HandoverNotifyIEsPresentAMFUENGAPID,
								AMFUENGAPID: &ngapType.AMFUENGAPID{Value: ranUe.AmfUeNgapId},
							},
						},
						{
							Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
							Value: ngapType.HandoverNotifyIEsValue{
								Present:     ngapType.HandoverNotifyIEsPresentRANUENGAPID,
								RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUe.RanUeNgapId},
							},
						},
					},
				}},
			},
		},
	})

	if seen[amfUe] != 1 {
		t.Errorf("a Handover Notify reported the location %d times, want 1", seen[amfUe])
	}
}

func TestPDUSessionResourceModifyIndicationReportsTheLocation(t *testing.T) {
	seen := observeLocationUpdates(t)
	ran, ranUe, amfUe := newReleasingUe(t, "imsi-208930100009984", context.UeContextN2NormalRelease, 984)
	ran.Conn = &ngaputil.TestConn{}

	HandlePDUSessionResourceModifyIndication(ctxt.Background(), ran, &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodePDUSessionResourceModifyIndication},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentPDUSessionResourceModifyIndication,
				PDUSessionResourceModifyIndication: &ngapType.PDUSessionResourceModifyIndication{
					ProtocolIEs: ngapType.ProtocolIEContainerPDUSessionResourceModifyIndicationIEs{
						List: []ngapType.PDUSessionResourceModifyIndicationIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Value: ngapType.PDUSessionResourceModifyIndicationIEsValue{
									Present:     ngapType.PDUSessionResourceModifyIndicationIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: ranUe.AmfUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Value: ngapType.PDUSessionResourceModifyIndicationIEsValue{
									Present:     ngapType.PDUSessionResourceModifyIndicationIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUe.RanUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDPDUSessionResourceModifyListModInd},
								Value: ngapType.PDUSessionResourceModifyIndicationIEsValue{
									Present:                            ngapType.PDUSessionResourceModifyIndicationIEsPresentPDUSessionResourceModifyListModInd,
									PDUSessionResourceModifyListModInd: &ngapType.PDUSessionResourceModifyListModInd{},
								},
							},
						},
					},
				},
			},
		},
	})

	if seen[amfUe] != 1 {
		t.Errorf("a PDU Session Resource Modify Indication reported the location %d times, want 1", seen[amfUe])
	}
}
