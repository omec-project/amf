// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package callback

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func TestSendN1N2TransferFailureNotificationUsesExactCallbackURI(t *testing.T) {
	var receivedRequestURI string
	receivedBody := models.NewN1N2MsgTxfrFailureNotificationWithDefaults()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequestURI = r.URL.RequestURI()
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(receivedBody); err != nil {
			t.Fatalf("failed to decode callback body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	callbackURI := server.URL + "/n1n2/failure?token=abc"
	jsonData := models.NewN1N2MessageTransferReqData()
	jsonData.SetN1n2FailureTxfNotifURI(callbackURI)
	ue := &amf_context.AmfUe{
		N1N2Message: &amf_context.N1N2Message{
			Request: models.N1N2MessageTransferRequest{
				JsonData: jsonData,
			},
			Status:      models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE,
			ResourceUri: "/namf-comm/v1/n1-n2-messages/1",
		},
	}

	SendN1N2TransferFailureNotification(ue, models.N1N2MESSAGETRANSFERCAUSE_UE_NOT_RESPONDING)

	if receivedRequestURI != "/n1n2/failure?token=abc" {
		t.Fatalf("expected exact callback URI path, got %q", receivedRequestURI)
	}
	if receivedBody.GetN1n2MsgDataUri() != "/namf-comm/v1/n1-n2-messages/1" {
		t.Fatalf("expected resource URI to be preserved, got %q", receivedBody.GetN1n2MsgDataUri())
	}
	if ue.N1N2Message != nil {
		t.Fatal("expected N1N2 message state to be cleared after successful callback")
	}
}

func TestSendN1MessageNotifyUsesExactCallbackURI(t *testing.T) {
	var receivedRequestURI string
	receivedRequest := models.NewN1MessageNotifyRequestWithDefaults()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequestURI = r.URL.RequestURI()
		defer r.Body.Close()
		requestBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		if !strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
			t.Fatalf("expected multipart content type, got %q", r.Header.Get("Content-Type"))
		}
		if err := openapi.Decode(receivedRequest, requestBody, r.Header.Get("Content-Type")); err != nil {
			t.Fatalf("failed to decode N1 message notify request: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	callbackURI := server.URL + "/n1-message/notify?subscription=7"
	subscription := models.NewUeN1N2InfoSubscriptionCreateData()
	subscription.SetN1NotifyCallbackUri(callbackURI)
	subscription.SetN1MessageClass(models.N1MESSAGECLASS_UPDP)

	ue := &amf_context.AmfUe{}
	ue.N1N2MessageSubscription.Store(int64(7), *subscription)

	SendN1MessageNotify(ue, models.N1MESSAGECLASS_UPDP, []byte{0x01, 0x02, 0x03}, nil)

	if receivedRequestURI != "/n1-message/notify?subscription=7" {
		t.Fatalf("expected exact callback URI path, got %q", receivedRequestURI)
	}
	jsonData := receivedRequest.GetJsonData()
	if jsonData.N1NotifySubscriptionId == nil || *jsonData.N1NotifySubscriptionId != "7" {
		t.Fatalf("expected subscription id 7, got %+v", jsonData.N1NotifySubscriptionId)
	}
	receivedN1Message := receivedRequest.GetBinaryDataN1Message()
	if receivedN1Message == nil {
		t.Fatal("expected binary N1 message to be decoded")
	}
	receivedN1Bytes, err := io.ReadAll(receivedN1Message)
	if err != nil {
		t.Fatalf("failed to read decoded binary N1 message: %v", err)
	}
	if !bytes.Equal(receivedN1Bytes, []byte{0x01, 0x02, 0x03}) {
		t.Fatalf("unexpected binary N1 payload %v", receivedN1Bytes)
	}
}

func TestSendAmfStatusChangeNotifyUsesExactCallbackURI(t *testing.T) {
	var receivedRequestURI string
	receivedBody := models.NewAmfStatusChangeNotificationWithDefaults()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequestURI = r.URL.RequestURI()
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(receivedBody); err != nil {
			t.Fatalf("failed to decode callback body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	guami := models.Guami{PlmnId: models.PlmnIdNid{Mcc: "001", Mnc: "01"}, AmfId: "cafe01"}
	subscription := models.NewSubscriptionDataAmf(server.URL + "/status/change?tracking=1")
	subscription.SetGuamiList([]models.Guami{guami})

	amfSelf := amf_context.AMF_Self()
	amfSelf.AMFStatusSubscriptions.Store("test-subscription", *subscription)
	defer amfSelf.AMFStatusSubscriptions.Delete("test-subscription")

	SendAmfStatusChangeNotify(models.STATUSCHANGE_AMF_UNAVAILABLE, []models.Guami{guami})

	if receivedRequestURI != "/status/change?tracking=1" {
		t.Fatalf("expected exact callback URI path, got %q", receivedRequestURI)
	}
	if len(receivedBody.GetAmfStatusInfoList()) != 1 {
		t.Fatalf("expected one status info entry, got %d", len(receivedBody.GetAmfStatusInfoList()))
	}
	if receivedBody.GetAmfStatusInfoList()[0].GetStatusChange() != models.STATUSCHANGE_AMF_UNAVAILABLE {
		t.Fatalf("unexpected status change %q", receivedBody.GetAmfStatusInfoList()[0].GetStatusChange())
	}
}
