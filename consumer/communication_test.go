// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path"
	"strings"
	"testing"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
)

func TestUEContextTransferRequestSendsRegistrationRequestAsMultipart(t *testing.T) {
	expectedRegistrationRequest := encodeRegistrationRequest(t)

	var receivedMethod string
	var receivedPath string
	var receivedMediaType string
	var receivedTransferData models.UeContextTransferReqData
	var receivedN1Message []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path

		parts, mediaType := readNamedMultipartRequestParts(t, r, "binaryDataN1Message")
		receivedMediaType = mediaType

		if err := json.Unmarshal(parts["jsonData"], &receivedTransferData); err != nil {
			t.Fatalf("failed to decode jsonData part: %v", err)
		}
		receivedN1Message = parts["binaryDataN1Message"]

		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{}`)); err != nil {
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
	if receivedTransferData.GetReason() != models.TRANSFERREASON_INIT_REG {
		t.Fatalf("expected transfer reason %s, got %s", models.TRANSFERREASON_INIT_REG, receivedTransferData.GetReason())
	}
	if receivedTransferData.GetAccessType() != models.ACCESSTYPE__3_GPP_ACCESS {
		t.Fatalf("expected access type %s, got %s", models.ACCESSTYPE__3_GPP_ACCESS, receivedTransferData.GetAccessType())
	}
	if receivedTransferData.RegRequest == nil {
		t.Fatal("expected registration request container")
	}
	if receivedTransferData.RegRequest.N1MessageContent.ContentId != "n1Msg" {
		t.Fatalf("expected N1 content id n1Msg, got %s", receivedTransferData.RegRequest.N1MessageContent.ContentId)
	}
	if !bytes.Equal(receivedN1Message, expectedRegistrationRequest) {
		t.Fatalf("expected N1 message payload %v, got %v", expectedRegistrationRequest, receivedN1Message)
	}
}

func encodeRegistrationRequest(t *testing.T) []byte {
	t.Helper()

	registrationRequest := nasMessage.NewRegistrationRequest(0)
	var buffer bytes.Buffer
	registrationRequest.EncodeRegistrationRequest(&buffer)
	return buffer.Bytes()
}

func readNamedMultipartRequestParts(t *testing.T, r *http.Request, expectedPart string) (map[string][]byte, string) {
	t.Helper()

	mediaType, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatalf("failed to parse content type: %v", err)
	}
	if boundary := params["boundary"]; mediaType == "" || boundary == "" {
		t.Fatalf("expected multipart content type with boundary, got %q", r.Header.Get("Content-Type"))
	}

	reader := multipart.NewReader(r.Body, params["boundary"])
	parts := make(map[string][]byte)
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read multipart part: %v", err)
		}

		body, err := io.ReadAll(part)
		if err != nil {
			t.Fatalf("failed to read part %q: %v", part.FormName(), err)
		}
		parts[part.FormName()] = body
	}

	if _, ok := parts["jsonData"]; !ok {
		t.Fatal("expected jsonData multipart part")
	}
	if _, ok := parts[expectedPart]; !ok {
		t.Fatalf("expected %s multipart part", expectedPart)
	}

	return parts, mediaType
}
