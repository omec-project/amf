// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/omec-project/openapi/v2/models"
)

// pendingN2OnlyMessage is the shape a DDN-triggered page stores: N2 information and no
// N1 message, which is exactly the case that used to make a stored context unreadable.
// These JSON keys are repeated across the tables below; goconst asks for names.
const (
	n2InfoContainerKey = "n2InfoContainer"
	n2InformationClass = "n2InformationClass"
)

func pendingN2OnlyMessage() *N1N2Message {
	jsonData := models.NewN1N2MessageTransferReqData()
	jsonData.SetPduSessionId(1)
	jsonData.SetN2InfoContainer(*models.NewN2InfoContainer(models.N2INFORMATIONCLASS_SM))

	request := models.NewN1N2MessageTransferRequest()
	request.SetJsonData(*jsonData)

	return &N1N2Message{
		Request: *request,
		Status:  models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE,
		N2Info:  []byte{0x01, 0x02, 0x03},
	}
}

func ueWithPendingMessage(t *testing.T) *AmfUe {
	t.Helper()

	ue := &AmfUe{}
	ue.init()
	ue.Supi = "imsi-208930100007488"
	ue.N1N2Message = pendingN2OnlyMessage()

	return ue
}

// The regression: storing a context wrote optional containers carrying empty 3GPP enum
// values, and the strict decoders refuse those, so the whole context could not be read
// back. The AMF then reported a UE that was present in the database as absent.
func TestStoredContextWithPendingMessageRoundTrips(t *testing.T) {
	ue := ueWithPendingMessage(t)

	stored, err := sonic.Marshal(ue)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Decode the way DbFetch does, so the read-path tolerance is part of the test: a
	// partly-populated context carries other empty enum values too -- ngKsi.tsc on a UE
	// stored before authentication finished, for one.
	var asStored map[string]any
	if err := sonic.Unmarshal(stored, &asStored); err != nil {
		t.Fatalf("re-read stored form: %v", err)
	}

	restored := &AmfUe{}
	restored.init()

	if err := sonic.Unmarshal(stored, restored); err != nil {
		dropEmptyEnumValues(asStored)

		relaxed, marshalErr := sonic.Marshal(asStored)
		if marshalErr != nil {
			t.Fatalf("marshal relaxed form: %v", marshalErr)
		}

		if err := sonic.Unmarshal(relaxed, restored); err != nil {
			t.Fatalf("a stored context must be readable, got: %v", err)
		}
	}

	if restored.Supi != ue.Supi {
		t.Errorf("restored SUPI = %q, want %q", restored.Supi, ue.Supi)
	}

	// The payload is the point: a restored message whose content did not survive
	// produces a PDU Session Resource Setup with an empty transfer, which is how a paged
	// UE came back with no user plane.
	if restored.N1N2Message == nil {
		t.Fatal("the restored context carries no pending message")
	}

	if got := string(restored.N1N2Message.N2Info); got != string(ue.N1N2Message.N2Info) {
		t.Errorf("restored N2 information = %x, want %x", restored.N1N2Message.N2Info, ue.N1N2Message.N2Info)
	}

	if _, n2Info, err := restored.N1N2Message.Payloads(); err != nil {
		t.Errorf("Payloads on a restored message: %v", err)
	} else if string(n2Info) != string(ue.N1N2Message.N2Info) {
		t.Errorf("Payloads N2 information = %x, want %x", n2Info, ue.N1N2Message.N2Info)
	}
}

// Storing shared the live JsonData pointer and replaced its containers with empty ones,
// so the act of storing a context destroyed the pending message inside it.
func TestStoringDoesNotDestroyThePendingMessage(t *testing.T) {
	ue := ueWithPendingMessage(t)

	if _, err := sonic.Marshal(ue); err != nil {
		t.Fatalf("marshal: %v", err)
	}

	container := ue.N1N2Message.Request.JsonData.N2InfoContainer
	if container == nil {
		t.Fatal("the live pending message lost its N2 container while being stored")
	}

	if got := container.GetN2InformationClass(); got != models.N2INFORMATIONCLASS_SM {
		t.Errorf("N2 information class = %q after storing, want %q", got, models.N2INFORMATIONCLASS_SM)
	}
}

func TestSerialisablePendingMessageOmitsContainersWithNoClass(t *testing.T) {
	jsonData := models.NewN1N2MessageTransferReqData()
	jsonData.SetN1MessageContainer(*models.NewN1MessageContainerWithDefaults())
	jsonData.SetN2InfoContainer(*models.NewN2InfoContainer(models.N2INFORMATIONCLASS_SM))

	request := models.NewN1N2MessageTransferRequest()
	request.SetJsonData(*jsonData)

	stored := serialisablePendingMessage(&N1N2Message{Request: *request})
	if stored == nil {
		t.Fatal("serialisablePendingMessage returned nil for a non-nil message")
	}

	if stored.Request.JsonData.N1MessageContainer != nil {
		t.Error("an N1 container with no class must be omitted, not stored empty")
	}

	if stored.Request.JsonData.N2InfoContainer == nil {
		t.Error("an N2 container with a class must be kept")
	}
}

// Records written before the fix are still in deployed databases, and a UE that had ever
// been paged stayed unreachable until it re-registered.
func TestDropEmptyEnumValuesHealsLegacyRecords(t *testing.T) {
	doc := map[string]any{
		"customFieldsAmfUe": map[string]any{
			"n1n2Msg": map[string]any{
				"Request": map[string]any{
					"jsonData": map[string]any{
						"n1MessageContainer": map[string]any{"n1MessageClass": ""},
						n2InfoContainerKey:   map[string]any{n2InformationClass: ""},
					},
				},
			},
		},
	}

	dropEmptyEnumValues(doc)

	jsonData := doc["customFieldsAmfUe"].(map[string]any)["n1n2Msg"].(map[string]any)["Request"].(map[string]any)["jsonData"].(map[string]any)

	// The container may remain as an empty object -- that decodes cleanly. What must be
	// gone is the empty class value, which is what a strict decoder refuses.
	for container, classField := range map[string]string{
		"n1MessageContainer": "n1MessageClass",
		n2InfoContainerKey:   "n2InformationClass",
	} {
		held, present := jsonData[container].(map[string]any)
		if !present {
			continue
		}

		if _, stillThere := held[classField]; stillThere {
			t.Errorf("%s.%s was empty and must have been dropped", container, classField)
		}
	}
}

func TestDropEmptyEnumValuesKeepsRealContent(t *testing.T) {
	doc := map[string]any{
		"customFieldsAmfUe": map[string]any{
			"n1n2Msg": map[string]any{
				"Request": map[string]any{
					"jsonData": map[string]any{
						n2InfoContainerKey: map[string]any{n2InformationClass: "SM"},
					},
				},
			},
		},
	}

	dropEmptyEnumValues(doc)

	jsonData := doc["customFieldsAmfUe"].(map[string]any)["n1n2Msg"].(map[string]any)["Request"].(map[string]any)["jsonData"].(map[string]any)
	if _, present := jsonData[n2InfoContainerKey]; !present {
		t.Error("a container carrying a class must be left alone")
	}
}

// A context whose RanUe has no RAN yet, or none any more, must still be storable.
func TestMarshalToleratesARanUeWithNoRan(t *testing.T) {
	ue := ueWithPendingMessage(t)
	ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] = &RanUe{RanUeNgapId: 7, AmfUeNgapId: 9}

	if _, err := sonic.Marshal(ue); err != nil {
		t.Fatalf("storing a context with no RAN attached must not fail: %v", err)
	}
}
