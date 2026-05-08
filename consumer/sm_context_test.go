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
	"os"
	"strings"
	"testing"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/models"
)

const updateSmContextPath = "/nsmf-pdusession/v1/sm-contexts/ctx-ref/modify"

func TestSendUpdateSmContextRequestSendsN2InfoAsMultipart(t *testing.T) {
	expectedN2Info := []byte{0x01, 0x02, 0x03, 0x04}
	n2SmInfoType := models.N2SMINFOTYPE_PDU_RES_SETUP_RSP
	updateData := models.SmContextUpdateData{
		N2SmInfoType: &n2SmInfoType,
		N2SmInfo: &models.RefToBinaryData{
			ContentId: "N2SmInfo",
		},
	}

	var receivedMethod string
	var receivedPath string
	var receivedMediaType string
	var receivedUpdateData models.SmContextUpdateData
	var receivedN2Info []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedMethod = r.Method
		receivedPath = r.URL.Path

		parts, mediaType := readMultipartRequestParts(t, r)
		receivedMediaType = mediaType

		if err := json.Unmarshal(parts["jsonData"], &receivedUpdateData); err != nil {
			t.Fatalf("failed to decode jsonData part: %v", err)
		}
		receivedN2Info = parts["binaryDataN2SmInformation"]

		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Fatalf("failed to write response body: %v", err)
		}
	}))
	defer server.Close()

	smContext := amf_context.NewSmContext(10)
	smContext.SetSmContextRef("ctx-ref")
	smContext.SetSmfUri(server.URL)
	smContext.SetSmfID("smf-test")

	response, errorResponse, problemDetail, err := SendUpdateSmContextRequest(
		context.Background(),
		smContext,
		updateData,
		nil,
		expectedN2Info,
	)
	if err != nil {
		t.Fatalf("SendUpdateSmContextRequest returned error: %v", err)
	}
	if response == nil {
		t.Fatal("expected success response")
	}
	if errorResponse != nil {
		t.Fatalf("expected no error response, got %+v", errorResponse)
	}
	if problemDetail != nil {
		t.Fatalf("expected no problem detail, got %+v", problemDetail)
	}

	if receivedMethod != http.MethodPost {
		t.Fatalf("expected POST request, got %s", receivedMethod)
	}
	if receivedPath != updateSmContextPath {
		t.Fatalf("unexpected request path %s", receivedPath)
	}
	if !strings.HasPrefix(receivedMediaType, "multipart/") {
		t.Fatalf("expected multipart request, got %q", receivedMediaType)
	}
	if receivedUpdateData.GetN2SmInfoType() != models.N2SMINFOTYPE_PDU_RES_SETUP_RSP {
		t.Fatalf("expected N2 SM info type %s, got %s", models.N2SMINFOTYPE_PDU_RES_SETUP_RSP, receivedUpdateData.GetN2SmInfoType())
	}
	n2SmInfo := receivedUpdateData.GetN2SmInfo()
	if n2SmInfo.ContentId != "N2SmInfo" {
		t.Fatalf("expected N2 content id N2SmInfo, got %s", n2SmInfo.ContentId)
	}
	if !bytes.Equal(receivedN2Info, expectedN2Info) {
		t.Fatalf("expected N2 payload %v, got %v", expectedN2Info, receivedN2Info)
	}
	if response.JsonData == nil {
		t.Fatal("expected response JsonData to be set")
	}
}

func TestSendUpdateSmContextRequestHandlesEmptySuccessBody(t *testing.T) {
	updateData := models.SmContextUpdateData{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != updateSmContextPath {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	smContext := amf_context.NewSmContext(10)
	smContext.SetSmContextRef("ctx-ref")
	smContext.SetSmfUri(server.URL)
	smContext.SetSmfID("smf-test")

	response, errorResponse, problemDetail, err := SendUpdateSmContextRequest(
		context.Background(),
		smContext,
		updateData,
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("SendUpdateSmContextRequest returned error: %v", err)
	}
	if response == nil {
		t.Fatal("expected success response")
	}
	if response.JsonData != nil {
		t.Fatalf("expected empty JsonData for empty success body, got %+v", response.JsonData)
	}
	if errorResponse != nil {
		t.Fatalf("expected no error response, got %+v", errorResponse)
	}
	if problemDetail != nil {
		t.Fatalf("expected no problem detail, got %+v", problemDetail)
	}
}

func TestSendUpdateSmContextRequestParsesMultipartSuccessResponse(t *testing.T) {
	expectedN1 := []byte{0x11, 0x22, 0x33}
	expectedN2 := []byte{0xaa, 0xbb, 0xcc, 0xdd}
	n2SmInfoType := models.N2SMINFOTYPE_PDU_RES_REL_CMD
	upCnxState := models.UPCNXSTATE_DEACTIVATED

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != updateSmContextPath {
			t.Fatalf("unexpected request path %s", r.URL.Path)
		}

		n1File := writeTempFile(t, expectedN1)
		defer os.Remove(n1File.Name())
		defer n1File.Close()

		n2File := writeTempFile(t, expectedN2)
		defer os.Remove(n2File.Name())
		defer n2File.Close()

		multipartBody := &bytes.Buffer{}
		writer := multipart.NewWriter(multipartBody)

		jsonData, err := json.Marshal(models.SmContextUpdatedData{
			N2SmInfoType: &n2SmInfoType,
			UpCnxState:   &upCnxState,
			N1SmMsg: &models.RefToBinaryData{
				ContentId: "PDUSessionReleaseCommand",
			},
			N2SmInfo: &models.RefToBinaryData{
				ContentId: "PDUResourceReleaseCommand",
			},
		})
		if err != nil {
			t.Fatalf("failed to marshal jsonData: %v", err)
		}

		jsonPart, err := writer.CreateFormField("jsonData")
		if err != nil {
			t.Fatalf("failed to create jsonData part: %v", err)
		}
		if _, err = jsonPart.Write(jsonData); err != nil {
			t.Fatalf("failed to write jsonData part: %v", err)
		}

		n1Part, err := writer.CreateFormField("binaryDataN1SmMessage")
		if err != nil {
			t.Fatalf("failed to create N1 part: %v", err)
		}
		if _, err = n1Part.Write(expectedN1); err != nil {
			t.Fatalf("failed to write N1 part: %v", err)
		}

		n2Part, err := writer.CreateFormField("binaryDataN2SmInformation")
		if err != nil {
			t.Fatalf("failed to create N2 part: %v", err)
		}
		if _, err = n2Part.Write(expectedN2); err != nil {
			t.Fatalf("failed to write N2 part: %v", err)
		}

		if err = writer.Close(); err != nil {
			t.Fatalf("failed to close multipart writer: %v", err)
		}
		contentType := "multipart/related; boundary=" + writer.Boundary()
		w.Header().Set("Content-Type", contentType)
		if _, err = w.Write(multipartBody.Bytes()); err != nil {
			t.Fatalf("failed to write multipart response body: %v", err)
		}
	}))
	defer server.Close()

	smContext := amf_context.NewSmContext(10)
	smContext.SetSmContextRef("ctx-ref")
	smContext.SetSmfUri(server.URL)
	smContext.SetSmfID("smf-test")

	response, errorResponse, problemDetail, err := SendUpdateSmContextRequest(
		context.Background(),
		smContext,
		models.SmContextUpdateData{},
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf("SendUpdateSmContextRequest returned error: %v", err)
	}
	if errorResponse != nil {
		t.Fatalf("expected no error response, got %+v", errorResponse)
	}
	if problemDetail != nil {
		t.Fatalf("expected no problem detail, got %+v", problemDetail)
	}
	if response == nil {
		t.Fatal("expected success response")
	}
	if response.JsonData == nil {
		t.Fatal("expected JsonData to be set")
	}
	if response.JsonData.GetN2SmInfoType() != models.N2SMINFOTYPE_PDU_RES_REL_CMD {
		t.Fatalf("expected N2 SM info type %s, got %s", models.N2SMINFOTYPE_PDU_RES_REL_CMD, response.JsonData.GetN2SmInfoType())
	}

	gotN1, err := io.ReadAll(response.GetBinaryDataN1SmMessage())
	if err != nil {
		t.Fatalf("failed to read returned N1 message: %v", err)
	}
	if !bytes.Equal(gotN1, expectedN1) {
		t.Fatalf("expected N1 payload %v, got %v", expectedN1, gotN1)
	}

	gotN2, err := io.ReadAll(response.GetBinaryDataN2SmInformation())
	if err != nil {
		t.Fatalf("failed to read returned N2 information: %v", err)
	}
	if !bytes.Equal(gotN2, expectedN2) {
		t.Fatalf("expected N2 payload %v, got %v", expectedN2, gotN2)
	}
}

func writeTempFile(t *testing.T, payload []byte) *os.File {
	t.Helper()

	tmpFile, err := os.CreateTemp("", "sm-context-test")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	if _, err = tmpFile.Write(payload); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to write temp file: %v", err)
	}
	if _, err = tmpFile.Seek(0, io.SeekStart); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		t.Fatalf("failed to rewind temp file: %v", err)
	}
	return tmpFile
}

func readMultipartRequestParts(t *testing.T, r *http.Request) (map[string][]byte, string) {
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
	if _, ok := parts["binaryDataN2SmInformation"]; !ok {
		t.Fatal("expected binaryDataN2SmInformation multipart part")
	}

	return parts, mediaType
}
