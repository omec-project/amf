// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	"bytes"
	ctxt "context"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/omec-project/amf/context"
	ngaputil "github.com/omec-project/amf/ngap/util"
	libngap "github.com/omec-project/ngap/v2"
	"github.com/omec-project/ngap/v2/aper"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
)

// pathSwitchRequestFor builds the Path Switch Request a target RAN sends for ranUe, listing it
// under ranUeNgapID and asking for one PDU session to be switched.
func pathSwitchRequestFor(ranUe *context.RanUe, ranUeNgapID int64, pduSessionID int64) *ngapType.NGAPPDU {
	return &ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodePathSwitchRequest},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentPathSwitchRequest,
				PathSwitchRequest: &ngapType.PathSwitchRequest{
					ProtocolIEs: ngapType.ProtocolIEContainerPathSwitchRequestIEs{
						List: []ngapType.PathSwitchRequestIEs{
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Value: ngapType.PathSwitchRequestIEsValue{
									Present:     ngapType.PathSwitchRequestIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUeNgapID},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDSourceAMFUENGAPID},
								Value: ngapType.PathSwitchRequestIEsValue{
									Present:           ngapType.PathSwitchRequestIEsPresentSourceAMFUENGAPID,
									SourceAMFUENGAPID: &ngapType.AMFUENGAPID{Value: ranUe.AmfUeNgapId},
								},
							},
							{
								Id: ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDPDUSessionResourceToBeSwitchedDLList},
								Value: ngapType.PathSwitchRequestIEsValue{
									Present: ngapType.PathSwitchRequestIEsPresentPDUSessionResourceToBeSwitchedDLList,
									PDUSessionResourceToBeSwitchedDLList: &ngapType.PDUSessionResourceToBeSwitchedDLList{
										List: []ngapType.PDUSessionResourceToBeSwitchedDLItem{{
											PDUSessionID:              ngapType.PDUSessionID{Value: pduSessionID},
											PathSwitchRequestTransfer: aper.OctetString{0x00},
										}},
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

// smfRemovingDuringTheSwitch returns the URI of an SMF that, while the AMF waits for it to switch
// a session, removes ranUe, as a release on another goroutine would, and then switches the
// session, which is what lets the AMF go on to switch the RanUe to the target RAN.
func smfRemovingDuringTheSwitch(t *testing.T, ranUe *context.RanUe) string {
	t.Helper()

	smf := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err := ranUe.Remove(); err != nil {
			t.Errorf("removing the RanUe during the switch: %v", err)
		}

		jsonData, err := json.Marshal(models.SmContextUpdatedData{
			N2SmInfoType: models.N2SMINFOTYPE_PATH_SWITCH_REQ_ACK.Ptr(),
			N2SmInfo:     models.NewRefToBinaryData("PathSwitchRequestAcknowledgeTransfer"),
		})
		if err != nil {
			t.Errorf("encoding the SMF's answer: %v", err)
			return
		}
		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)
		for name, content := range map[string][]byte{
			"jsonData":                  jsonData,
			"binaryDataN2SmInformation": {0x00},
		} {
			part, err := writer.CreateFormField(name)
			if err != nil {
				t.Errorf("creating the %s part: %v", name, err)
				return
			}
			if _, err := part.Write(content); err != nil {
				t.Errorf("writing the %s part: %v", name, err)
				return
			}
		}
		if err := writer.Close(); err != nil {
			t.Errorf("closing the SMF's answer: %v", err)
			return
		}
		w.Header().Set("Content-Type", "multipart/related; boundary="+writer.Boundary())
		if _, err := w.Write(body.Bytes()); err != nil {
			t.Errorf("writing the SMF's answer: %v", err)
		}
	}))
	t.Cleanup(smf.Close)

	return smf.URL
}

// A RanUe removed while the SMF switches its session is not switched to the target RAN, which
// is answered with a Path Switch Request Failure rather than an acknowledgement for a UE the AMF
// no longer holds.
func TestAPathSwitchForARanUeRemovedMeanwhileFails(t *testing.T) {
	_, ranUe, amfUe := newReleasingUe(t, "imsi-208930100009970", context.UeContextN2NormalRelease, 970)
	// What its registration gave it, and an acknowledgement carries.
	amfUe.UESecurityCapability.SetLen(2)
	amfUe.SetAllowedNssai(models.ACCESSTYPE__3_GPP_ACCESS, []models.AllowedSnssai{*models.NewAllowedSnssai(*models.NewSnssai(1))})
	smContext := context.NewSmContext(1)
	smContext.SetSmContextRef("switching-session")
	smContext.SetSmfUri(smfRemovingDuringTheSwitch(t, ranUe))
	smContext.SetSmfID("smf-test")
	amfUe.StoreSmContext(1, smContext)

	target := context.NewAmfRanDefault()
	target.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	conn := &ngaputil.TestConn{}
	target.Conn = conn

	HandlePathSwitchRequest(ctxt.Background(), target, pathSwitchRequestFor(ranUe, 971, 1))

	sent := conn.Snapshot()
	if len(sent) == 0 {
		t.Fatal("the target RAN received nothing")
	}
	pdu, err := libngap.Decoder(sent)
	if err != nil {
		t.Fatalf("decoding what the target RAN received: %v", err)
	}
	if pdu.Present != ngapType.NGAPPDUPresentUnsuccessfulOutcome ||
		pdu.UnsuccessfulOutcome.ProcedureCode.Value != ngapType.ProcedureCodePathSwitchRequest {
		t.Errorf("the target RAN received an NGAP PDU of type %d, want a Path Switch Request Failure (%d)",
			pdu.Present, ngapType.NGAPPDUPresentUnsuccessfulOutcome)
	}
	if target.RanUeFindByRanUeNgapIDLocal(971) != nil {
		t.Error("the removed RanUe is on the target RAN")
	}
}

// withoutPathSwitchIEs returns pdu without the IEs listed in ids.
func withoutPathSwitchIEs(pdu *ngapType.NGAPPDU, ids ...int64) *ngapType.NGAPPDU {
	request := pdu.InitiatingMessage.Value.PathSwitchRequest
	kept := request.ProtocolIEs.List[:0:0]
	for _, ie := range request.ProtocolIEs.List {
		drop := false
		for _, id := range ids {
			drop = drop || ie.Id.Value == id
		}
		if !drop {
			kept = append(kept, ie)
		}
	}
	request.ProtocolIEs.List = kept
	return pdu
}

// A Path Switch Request missing a mandatory identifier cannot be answered with a Path Switch Request
// Failure, so it is answered with an Error Indication (abstract syntax error, reject) that carries
// only the identifiers that were received, and the UE's RAN association is left alone.
func TestPathSwitchRequestMissingMandatoryIdsIsAnsweredWithErrorIndication(t *testing.T) {
	const newRanUeNgapID = 981
	tests := []struct {
		name          string
		omit          []int64
		wantAmfUeId   bool
		wantRanUeId   bool
		wantRanUeNgap int64
	}{
		{"without RAN UE NGAP ID", []int64{ngapType.ProtocolIEIDRANUENGAPID}, true, false, 0},
		{"without source AMF UE NGAP ID", []int64{ngapType.ProtocolIEIDSourceAMFUENGAPID}, false, true, newRanUeNgapID},
		{"without both", []int64{ngapType.ProtocolIEIDRANUENGAPID, ngapType.ProtocolIEIDSourceAMFUENGAPID}, false, false, 0},
	}
	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sourceRan, ranUe, _ := newReleasingUe(t, fmt.Sprintf("imsi-20893010000998%d", i),
				context.UeContextN2NormalRelease, int64(980+10*i))
			ranUeNgapIDBefore := ranUe.RanUeNgapId

			target := context.NewAmfRanDefault()
			target.AnType = models.ACCESSTYPE__3_GPP_ACCESS
			conn := &ngaputil.TestConn{}
			target.Conn = conn

			pdu := withoutPathSwitchIEs(pathSwitchRequestFor(ranUe, newRanUeNgapID, 1), tt.omit...)
			HandlePathSwitchRequest(ctxt.Background(), target, pdu)

			sent := conn.Snapshot()
			if len(sent) == 0 {
				t.Fatal("the target RAN received nothing")
			}
			got, err := libngap.Decoder(sent)
			if err != nil {
				t.Fatalf("decoding what the target RAN received: %v", err)
			}
			if got.Present != ngapType.NGAPPDUPresentInitiatingMessage ||
				got.InitiatingMessage.ProcedureCode.Value != ngapType.ProcedureCodeErrorIndication {
				t.Fatalf("the target RAN received PDU type %d, want an Error Indication", got.Present)
			}
			var amfUeID, ranUeID *int64
			var cause *ngapType.Cause
			for _, ie := range got.InitiatingMessage.Value.ErrorIndication.ProtocolIEs.List {
				switch ie.Id.Value {
				case ngapType.ProtocolIEIDAMFUENGAPID:
					amfUeID = &ie.Value.AMFUENGAPID.Value
				case ngapType.ProtocolIEIDRANUENGAPID:
					ranUeID = &ie.Value.RANUENGAPID.Value
				case ngapType.ProtocolIEIDCause:
					cause = ie.Value.Cause
				}
			}
			if cause == nil || cause.Present != ngapType.CausePresentProtocol ||
				cause.Protocol.Value != ngapType.CauseProtocolPresentAbstractSyntaxErrorReject {
				t.Errorf("cause = %+v, want protocol/abstract-syntax-error-reject", cause)
			}
			if (amfUeID != nil) != tt.wantAmfUeId || (amfUeID != nil && *amfUeID != ranUe.AmfUeNgapId) {
				t.Errorf("AMF UE NGAP ID in the Error Indication = %v, want present=%v (%d)",
					amfUeID, tt.wantAmfUeId, ranUe.AmfUeNgapId)
			}
			if (ranUeID != nil) != tt.wantRanUeId || (ranUeID != nil && *ranUeID != tt.wantRanUeNgap) {
				t.Errorf("RAN UE NGAP ID in the Error Indication = %v, want present=%v (%d)",
					ranUeID, tt.wantRanUeId, tt.wantRanUeNgap)
			}

			if ranUe.Ran != sourceRan {
				t.Error("the rejected request moved the RanUe to another RAN")
			}
			if ranUe.RanUeNgapId != ranUeNgapIDBefore {
				t.Errorf("RanUeNgapId = %d, want %d unchanged", ranUe.RanUeNgapId, ranUeNgapIDBefore)
			}
			if target.RanUeFindByRanUeNgapIDLocal(newRanUeNgapID) != nil {
				t.Error("the rejected request left a RanUe on the target RAN")
			}
		})
	}
}
