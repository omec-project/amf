// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	"bytes"
	ctxt "context"
	"encoding/json"
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
