// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

// drainedPayloadFile writes content to a temp file and reads it to EOF, which is the
// state the transfer procedure leaves the request in before storing it.
func drainedPayloadFile(t *testing.T, content []byte) *os.File {
	t.Helper()

	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write payload: %v", err)
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open payload: %v", err)
	}

	t.Cleanup(func() { f.Close() })

	if _, err := io.ReadAll(f); err != nil {
		t.Fatalf("drain payload: %v", err)
	}

	return f
}

func pendingMessageWithN2Container(t *testing.T, f *os.File, captured []byte) *N1N2Message {
	t.Helper()

	jsonData := models.NewN1N2MessageTransferReqData()
	jsonData.SetN2InfoContainer(*models.NewN2InfoContainer(models.N2INFORMATIONCLASS_SM))

	request := models.NewN1N2MessageTransferRequest()
	request.SetJsonData(*jsonData)
	request.SetBinaryDataN2Information(f)

	return &N1N2Message{Request: *request, N2Info: captured}
}

// A paged UE answers seconds after the message was stored, by which point the
// request's reader is at EOF and returns zero bytes with no error. Without the
// captured payload the AMF sent the RAN a PDU Session Resource Setup with an empty
// transfer, so the UE reconnected with no user plane.
func TestN1N2MessagePayloadsSurvivesDrainedReader(t *testing.T) {
	want := []byte{0xde, 0xad, 0xbe, 0xef}
	msg := pendingMessageWithN2Container(t, drainedPayloadFile(t, want), want)

	_, n2Info, err := msg.Payloads()
	if err != nil {
		t.Fatalf("Payloads: unexpected error %v", err)
	}

	if string(n2Info) != string(want) {
		t.Errorf("N2 information = %x, want %x", n2Info, want)
	}
}

// A declared container with nothing behind it must fail loudly rather than produce an
// empty item on the wire.
func TestN1N2MessagePayloadsRejectsDeclaredButEmptyContainer(t *testing.T) {
	msg := pendingMessageWithN2Container(t, drainedPayloadFile(t, []byte{0x01}), nil)

	if _, _, err := msg.Payloads(); err == nil {
		t.Fatal("Payloads: want an error for a declared N2 container with no content, got nil")
	}
}

// Nothing declared, nothing captured: not an error, just no payloads.
func TestN1N2MessagePayloadsAllowsNoContainers(t *testing.T) {
	request := models.NewN1N2MessageTransferRequest()
	request.SetJsonData(*models.NewN1N2MessageTransferReqData())

	msg := &N1N2Message{Request: *request}

	n1Msg, n2Info, err := msg.Payloads()
	if err != nil {
		t.Fatalf("Payloads: unexpected error %v", err)
	}

	if len(n1Msg) != 0 || len(n2Info) != 0 {
		t.Errorf("payloads = (%x, %x), want empty", n1Msg, n2Info)
	}
}
