// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/nas/v2/nasType"
	"github.com/omec-project/openapi/v2/models"
	"go.uber.org/zap"
)

// gsmPayload encodes a bare 5GSM message of the given type, which is all the
// Request-type gate inspects.
func gsmPayload(t *testing.T, msgType uint8) []byte {
	t.Helper()
	return []byte{
		nasMessage.Epd5GSSessionManagementMessage,
		0x0a, // PDU session identity
		0x00, // PTI
		msgType,
	}
}

func TestIsEstablishmentRequestFromUE(t *testing.T) {
	ue := &context.AmfUe{GmmLog: zap.NewNop().Sugar()}

	tests := []struct {
		name string
		msg  []byte
		want bool
	}{
		{"establishment request", gsmPayload(t, nas.MsgTypePDUSessionEstablishmentRequest), true},
		{"modification request", gsmPayload(t, nas.MsgTypePDUSessionModificationRequest), false},
		{"release request", gsmPayload(t, nas.MsgTypePDUSessionReleaseRequest), false},
		{"release complete", gsmPayload(t, nas.MsgTypePDUSessionReleaseComplete), false},
		{"empty payload", nil, false},
		{"undecodable payload", []byte{0xff}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isEstablishmentRequestFromUE(tc.msg, ue); got != tc.want {
				t.Fatalf("isEstablishmentRequestFromUE(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// A modification request carrying Request type "initial request" must not be mistaken
// for an establishment. Both the SM context delete and the duplicate-session release are
// gated on this discriminator, so a false here is what keeps an established session
// intact when a UE signals the wrong Request type.
func TestModificationRequestIsNotTreatedAsEstablishment(t *testing.T) {
	ue := &context.AmfUe{GmmLog: zap.NewNop().Sugar()}
	modReq := gsmPayload(t, nas.MsgTypePDUSessionModificationRequest)

	if isEstablishmentRequestFromUE(modReq, ue) {
		t.Fatal("a PDU session modification request must not be classified as an establishment request")
	}
}

// The discriminator is only useful if both gates consult it. This pins the observable
// consequence: an established session survives a modification request that carries the
// wrong Request type. The forwarding that follows is expected to fail in a unit test —
// there is no SMF — and the assertion is deliberately on the SM context, not the error.
func TestModificationRequestWithInitialRequestTypeKeepsSmContext(t *testing.T) {
	const pduID int32 = 10

	ue := &context.AmfUe{
		GmmLog: zap.NewNop().Sugar(),
		NASLog: zap.NewNop().Sugar(),
	}
	ue.SmContextList.Store(pduID, context.NewSmContext(pduID))

	ul := nasMessage.NewULNASTransport(0)
	ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
	ul.SetPduSessionID2Value(uint8(pduID))
	ul.RequestType = nasType.NewRequestType(nasMessage.ULNASTransportRequestTypeType)
	ul.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeInitialRequest)
	ul.SetPayloadContainerContents(gsmPayload(t, nas.MsgTypePDUSessionModificationRequest))

	// Forwarding cannot succeed without an SMF; the error is expected and is not what
	// this test asserts on.
	if err := transport5GSMMessage(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS, ul); err != nil {
		t.Logf("forwarding failed as expected with no SMF reachable: %v", err)
	}

	if _, ok := ue.SmContextFindByPDUSessionID(pduID); !ok {
		t.Fatal("SM context was discarded for a modification request carrying Request type " +
			"\"initial request\"; the established session must survive it")
	}
}
