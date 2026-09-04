// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	ctxt "context"
	"os"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/nas/v2/nasType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"go.uber.org/zap"
)

// The SMF can refuse a PDU session without attaching a NAS reject: TS 29.502 allows the error
// response to carry jsonData alone. Relaying that as a DL NAS transport would put an empty payload
// container in front of the UE, which is not a message it can act on - so both rejection paths
// check the payload before sending, and this file pins that they do.
//
// Reachability is what made these unpinned: the guards sit past a call into the consumer package,
// which is why the pre-existing table test skips its SMF cases. They are reached here by stubbing
// the consumer calls through the package vars gmm already uses for exactly this
// (sendReleaseSmContextRequest and friends), so no SMF or NGAP harness is involved.

// disableKafkaForRelayTest keeps PublishUeCtxtInfo out of the way: both relay paths call it, and it
// dereferences factory.AmfConfig.Configuration.KafkaInfo.EnableKafka, which is nil in a unit test.
// Mirrors the helper of the same shape in the ngap package's tests.
func disableKafkaForRelayTest(t *testing.T) {
	t.Helper()

	originalConfig := factory.AmfConfig.Configuration
	if originalConfig == nil {
		factory.AmfConfig.Configuration = &factory.Configuration{}
	}
	originalEnableKafka := factory.AmfConfig.Configuration.KafkaInfo.EnableKafka
	disabled := false
	factory.AmfConfig.Configuration.KafkaInfo.EnableKafka = &disabled
	t.Cleanup(func() {
		if originalConfig == nil {
			factory.AmfConfig.Configuration = nil

			return
		}
		factory.AmfConfig.Configuration = originalConfig
		factory.AmfConfig.Configuration.KafkaInfo.EnableKafka = originalEnableKafka
	})
}

// relayTestUe is the minimum a UE needs to reach either guard: a logger, and a RanUe for the
// access type, since the relay hands ue.GetRanUe(anType) to sendDLNASTransport.
func relayTestUe() *context.AmfUe {
	ue := &context.AmfUe{
		GmmLog:       zap.NewNop().Sugar(),
		RanUe:        make(map[models.AccessType]*context.RanUe),
		AllowedNssai: map[models.AccessType][]models.AllowedSnssai{},
	}
	ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] = &context.RanUe{AmfUe: ue}

	return ue
}

// n1TempFile writes payload to a temp file shaped like the one openapi.Decode creates for a
// multipart binary part. ReadAndCleanupBinaryTempFile removes it, so no cleanup is registered.
func n1TempFile(t *testing.T, payload []byte) *os.File {
	t.Helper()

	f, err := os.CreateTemp("", "n1smmessage")
	if err != nil {
		t.Fatalf("creating the temp file: %v", err)
	}
	if _, err = f.Write(payload); err != nil {
		t.Fatalf("writing the temp file: %v", err)
	}

	return f
}

// captureDLNASTransport replaces sendDLNASTransport for the duration of the test and returns the
// payloads it was called with.
func captureDLNASTransport(t *testing.T) *[][]byte {
	t.Helper()

	original := sendDLNASTransport
	sent := &[][]byte{}
	sendDLNASTransport = func(_ *context.RanUe, _ models.AccessType, _ uint8, nasPdu []byte,
		_ int32, _ uint8, _ *uint8, _ uint8,
	) {
		*sent = append(*sent, nasPdu)
	}
	t.Cleanup(func() { sendDLNASTransport = original })

	return sent
}

func updateRejection(n1 *os.File) *models.UpdateSmContext400Response {
	errResponse := models.NewUpdateSmContext400Response()
	errResponse.SetJsonData(models.SmContextUpdateError{
		Error: models.ExtProblemDetails{
			Status: openapi.PtrInt32(403),
			Cause:  openapi.PtrString("N1_SM_ERROR"),
		},
	})
	if n1 != nil {
		errResponse.SetBinaryDataN1SmMessage(n1)
	}

	return errResponse
}

// The modification path: forward5GSMMessageToSMF relays the reject the SMF returned, and sends
// nothing when there is no reject to relay.
func TestForwardRelaysOnlyARejectionThatCarriesAPayload(t *testing.T) {
	reject := []byte{0x2e, 0x0a, 0xca, 0x20}

	tests := []struct {
		name     string
		n1       *os.File
		wantSent [][]byte
	}{
		{
			name:     "a reject is relayed to the UE",
			n1:       n1TempFile(t, reject),
			wantSent: [][]byte{reject},
		},
		{
			// TS 29.502 allows an error response carrying jsonData alone.
			name:     "no N1 SM message: nothing is sent",
			n1:       nil,
			wantSent: nil,
		},
		{
			// A part that decoded to zero bytes is the same thing by a different route.
			name:     "an empty N1 SM message: nothing is sent",
			n1:       n1TempFile(t, nil),
			wantSent: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			disableKafkaForRelayTest(t)
			sent := captureDLNASTransport(t)
			originalUpdate := sendUpdateSmContextRequest
			t.Cleanup(func() { sendUpdateSmContextRequest = originalUpdate })
			sendUpdateSmContextRequest = func(_ ctxt.Context, _ *context.SmContext,
				_ models.SmContextUpdateData, _ []byte, _ []byte,
			) (*models.UpdateSmContext200Response, *models.UpdateSmContext400Response,
				*models.ProblemDetails, error,
			) {
				return nil, updateRejection(tc.n1), nil, nil
			}

			ue := relayTestUe()
			smContext := context.NewSmContext(10)

			if err := forward5GSMMessageToSMF(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS,
				10, smContext, nil); err != nil {
				t.Fatalf("forward5GSMMessageToSMF() = %v, want nil", err)
			}

			assertSent(t, *sent, tc.wantSent)
		})
	}
}

func assertSent(t *testing.T, got, want [][]byte) {
	t.Helper()

	if len(got) != len(want) {
		// The two directions are different defects, so they are reported as different things.
		why := "a reject the SMF did attach must reach the UE"
		if len(want) == 0 {
			why = "an SMF refusal with no reject attached must not reach the UE as an empty payload container"
		}
		t.Fatalf("sendDLNASTransport called %d time(s), want %d: %s", len(got), len(want), why)
	}
	for i := range want {
		if string(got[i]) != string(want[i]) {
			t.Errorf("sendDLNASTransport payload %d = %#x, want %#x", i, got[i], want[i])
		}
	}
}

func createRejection(n1 *os.File) *models.PostSmContexts400Response {
	errResponse := models.NewPostSmContexts400Response()
	errResponse.SetJsonData(models.SmContextCreateError{
		Error: models.ExtProblemDetails{
			Status: openapi.PtrInt32(403),
			Cause:  openapi.PtrString("N1_SM_ERROR"),
		},
	})
	if n1 != nil {
		errResponse.SetBinaryDataN1SmMessage(n1)
	}

	return errResponse
}

// establishmentRequestUL is an uplink NAS transport carrying a PDU session establishment request,
// shaped so the create path is the one taken: an initial request type, a payload container to
// forward, and a DNN, which keeps pickDNN off ue.ServingAMF.
func establishmentRequestUL(pduSessionID uint8) *nasMessage.ULNASTransport {
	ul := &nasMessage.ULNASTransport{}
	ul.PduSessionID2Value = nasType.NewPduSessionID2Value(nasMessage.ULNASTransportPduSessionID2ValueType)
	ul.SetPduSessionID2Value(pduSessionID)
	requestType := nasType.NewRequestType(nasMessage.ULNASTransportRequestTypeType)
	requestType.SetRequestTypeValue(nasMessage.ULNASTransportRequestTypeInitialRequest)
	ul.RequestType = requestType
	payloadContainer := nasType.PayloadContainer{}
	payloadContainer.SetLen(4)
	payloadContainer.SetPayloadContainerContents([]byte{0x2e, 0x01, 0x01, 0xc1})
	ul.PayloadContainer = payloadContainer
	dnn := nasType.NewDNN(nasMessage.ULNASTransportDNNType)
	dnn.SetLen(9)
	dnn.SetDNN([]byte("internet"))
	ul.DNN = dnn

	return ul
}

// The establishment path: transport5GSMMessage relays the reject the SMF returned, and sends
// nothing when the refusal carries no reject.
//
// The guard sits past consumer.SelectSmf and the create request, which is why the pre-existing
// table test skips its SMF cases. Both are stubbed through the package vars, so no SMF is needed.
func TestTransportRelaysOnlyARejectionThatCarriesAPayload(t *testing.T) {
	reject := []byte{0x2e, 0x0a, 0xc1, 0x46}

	tests := []struct {
		name     string
		n1       *os.File
		wantSent [][]byte
	}{
		{
			name:     "a reject is relayed to the UE",
			n1:       n1TempFile(t, reject),
			wantSent: [][]byte{reject},
		},
		{
			name:     "no N1 SM message: nothing is sent",
			n1:       nil,
			wantSent: nil,
		},
		{
			name:     "an empty N1 SM message: nothing is sent",
			n1:       n1TempFile(t, nil),
			wantSent: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			disableKafkaForRelayTest(t)
			sent := captureDLNASTransport(t)

			originalSelect, originalCreate := selectSmf, sendCreateSmContextRequest
			t.Cleanup(func() { selectSmf, sendCreateSmContextRequest = originalSelect, originalCreate })
			selectSmf = func(_ ctxt.Context, _ *context.AmfUe, _ models.AccessType, pduSessionID int32,
				_ models.Snssai, _ string,
			) (*context.SmContext, uint8, error) {
				return context.NewSmContext(pduSessionID), 0, nil
			}
			sendCreateSmContextRequest = func(_ ctxt.Context, _ *context.AmfUe, _ *context.SmContext,
				_ *models.RequestType, _ []byte,
			) (*models.PostSmContexts201Response, string, *models.PostSmContexts400Response,
				*models.ProblemDetails, error,
			) {
				return nil, "", createRejection(tc.n1), nil, nil
			}

			ue := relayTestUe()
			ue.AllowedNssai[models.ACCESSTYPE__3_GPP_ACCESS] = []models.AllowedSnssai{
				{AllowedSnssai: models.Snssai{Sst: 1}},
			}

			if err := transport5GSMMessage(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS,
				establishmentRequestUL(10)); err != nil {
				t.Fatalf("transport5GSMMessage() = %v, want nil", err)
			}

			assertSent(t, *sent, tc.wantSent)
		})
	}
}
