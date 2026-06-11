// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"strings"
	"testing"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func TestUEContextTransferRequestSendsRegistrationRequestAsMultipart(t *testing.T) {
	expectedRegistrationRequest := encodeRegistrationRequest(t)

	var receivedMethod string
	var receivedPath string
	var receivedMediaType string
	var receivedTransferRequest models.UEContextTransferRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path
		defer r.Body.Close()
		requestBody, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read request body: %v", err)
		}
		receivedMediaType = r.Header.Get("Content-Type")
		if decodeErr := openapi.Decode(&receivedTransferRequest, requestBody, receivedMediaType); decodeErr != nil {
			t.Fatalf("failed to decode transfer request: %v", decodeErr)
		}

		response := models.NewUEContextTransfer200Response()
		response.SetJsonData(models.UeContextTransferRspData{})
		payload := &bytes.Buffer{}
		contentType, encodeErr := openapi.MultipartEncode(response, payload)
		if encodeErr != nil {
			t.Fatalf("failed to encode multipart response: %v", encodeErr)
		}
		w.Header().Set("Content-Type", contentType)
		if _, err := w.Write(payload.Bytes()); err != nil {
			t.Fatalf("failed to write response body: %v", err)
		}
	}))
	defer server.Close()

	ue := &amf_context.AmfUe{
		TargetAmfUri:        server.URL,
		Supi:                "imsi-001010000000001",
		PlmnId:              models.PlmnId{Mcc: "001", Mnc: "01"},
		Guti:                "00101cafe00000001",
		RegistrationRequest: nasMessage.NewRegistrationRequest(0),
	}

	response, problemDetails, err := UEContextTransferRequest(
		context.Background(),
		ue,
		models.ACCESSTYPE__3_GPP_ACCESS,
		models.TRANSFERREASON_INIT_REG,
	)
	if err != nil {
		t.Fatalf("UEContextTransferRequest returned error: %v", err)
	}
	if problemDetails != nil {
		t.Fatalf("expected no problem details, got %+v", problemDetails)
	}
	if response == nil {
		t.Fatal("expected response to be set")
	}

	if receivedMethod != http.MethodPost {
		t.Fatalf("expected POST request, got %s", receivedMethod)
	}
	if !strings.HasPrefix(receivedMediaType, "multipart/") {
		t.Fatalf("expected multipart request, got %q", receivedMediaType)
	}
	if path.Base(receivedPath) != "transfer" {
		t.Fatalf("unexpected request path %s", receivedPath)
	}
	jsonData := receivedTransferRequest.GetJsonData()
	if jsonData.GetReason() != models.TRANSFERREASON_INIT_REG {
		t.Fatalf("expected transfer reason %s, got %s", models.TRANSFERREASON_INIT_REG, jsonData.GetReason())
	}
	if jsonData.GetAccessType() != models.ACCESSTYPE__3_GPP_ACCESS {
		t.Fatalf("expected access type %s, got %s", models.ACCESSTYPE__3_GPP_ACCESS, jsonData.GetAccessType())
	}
	if jsonData.RegRequest == nil {
		t.Fatal("expected registration request container")
	}
	if jsonData.RegRequest.N1MessageContent.GetContentId() != "n1Msg" {
		t.Fatalf("expected N1 content id n1Msg, got %s", jsonData.RegRequest.N1MessageContent.GetContentId())
	}
	receivedN1Message := receivedTransferRequest.GetBinaryDataN1Message()
	if receivedN1Message == nil {
		t.Fatal("expected binary N1 message part to be decoded")
	}
	receivedN1Bytes, err := io.ReadAll(receivedN1Message)
	if err != nil {
		t.Fatalf("failed to read decoded binary N1 message: %v", err)
	}
	if !bytes.Equal(receivedN1Bytes, expectedRegistrationRequest) {
		t.Fatalf("expected N1 message payload %v, got %v", expectedRegistrationRequest, receivedN1Bytes)
	}
}

func TestUEContextTransferRequestDecodesMultipartSuccessResponse(t *testing.T) {
	binaryN2Info, err := createBinaryPayloadTempFile([]byte{0xde, 0xad, 0xbe, 0xef})
	if err != nil {
		t.Fatalf("failed to create binary N2 payload: %v", err)
	}
	defer cleanupBinaryPayloadTempFile(binaryN2Info)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := models.NewUEContextTransfer200Response()
		response.SetJsonData(models.UeContextTransferRspData{
			UeContext:         models.UeContext{Supi: openapi.PtrString("imsi-001010000000001")},
			UeRadioCapability: models.NewN2InfoContent(models.RefToBinaryData{ContentId: "n2Info"}),
		})
		response.SetBinaryDataN2Information(binaryN2Info)

		payload := &bytes.Buffer{}
		contentType, encodeErr := openapi.MultipartEncode(response, payload)
		if encodeErr != nil {
			t.Fatalf("failed to encode multipart response: %v", encodeErr)
		}
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(http.StatusOK)
		if _, err = w.Write(payload.Bytes()); err != nil {
			t.Fatalf("failed to write response body: %v", err)
		}
	}))
	defer server.Close()

	ue := &amf_context.AmfUe{
		TargetAmfUri:        server.URL,
		Supi:                "imsi-001010000000001",
		PlmnId:              models.PlmnId{Mcc: "001", Mnc: "01"},
		Guti:                "00101cafe00000001",
		RegistrationRequest: nasMessage.NewRegistrationRequest(0),
	}

	response, problemDetails, err := UEContextTransferRequest(
		context.Background(),
		ue,
		models.ACCESSTYPE__3_GPP_ACCESS,
		models.TRANSFERREASON_MOBI_REG,
	)
	if err != nil {
		t.Fatalf("UEContextTransferRequest returned error: %v", err)
	}
	if problemDetails != nil {
		t.Fatalf("expected no problem details, got %+v", problemDetails)
	}
	if response == nil {
		t.Fatal("expected response to be set")
	}
	ueContext := response.GetUeContext()
	if ueContext.Supi == nil || *ueContext.Supi != "imsi-001010000000001" {
		t.Fatalf("unexpected SUPI %+v", ueContext.Supi)
	}
	if !response.HasUeRadioCapability() {
		t.Fatal("expected UE radio capability to be preserved from multipart success response")
	}
	n2Info := response.GetUeRadioCapability()
	if n2Info.NgapData.ContentId != "n2Info" {
		t.Fatalf("unexpected content id %q", n2Info.NgapData.ContentId)
	}

	if _, err = os.Stat(binaryN2Info.Name()); err != nil {
		t.Fatalf("expected test binary file to remain available during response encoding: %v", err)
	}
}

func encodeRegistrationRequest(t *testing.T) []byte {
	t.Helper()

	registrationRequest := nasMessage.NewRegistrationRequest(0)
	var buffer bytes.Buffer
	registrationRequest.EncodeRegistrationRequest(&buffer)
	return buffer.Bytes()
}
