// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package message

import (
	"bytes"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
)

// relayingUe is the minimum RanUe SendDLNASTransport dereferences: a UE with an access type,
// a RAN, and the loggers every path on the way through writes to.
func relayingUe(t *testing.T) *context.RanUe {
	t.Helper()

	ran := context.NewAmfRanDefault()
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	ran.Log = logger.NgapLog

	amfUe := &context.AmfUe{
		GmmLog: logger.GmmLog,
		AllowedNssai: map[models.AccessType][]models.AllowedSnssai{
			models.ACCESSTYPE__3_GPP_ACCESS: {{AllowedSnssai: models.Snssai{Sst: 1}}},
		},
	}

	return &context.RanUe{
		AmfUe:       amfUe,
		Ran:         ran,
		AmfUeNgapId: 1,
		RanUeNgapId: 2,
		Log:         logger.NgapLog,
	}
}

// TestADownlinkUEPolicyContainerIsNoted: TS 33.128 clause 6.2.2.2.12 makes the record's
// uEPolicy the MANAGE UE POLICY COMMAND, which this AMF relays downlink from two separate N1N2
// relay paths. The record is produced on the UE's COMPLETE, so a COMMAND this hook misses is a
// record the COMPLETE cannot produce.
//
// The hook is on the path both relays share rather than beside the two call sites: a hook
// placed beside one caller is not inherited by the next.
func TestADownlinkUEPolicyContainerIsNoted(t *testing.T) {
	var got []byte
	reported := 0

	restore := noteUEPolicyCommand
	noteUEPolicyCommand = func(_ *context.AmfUe, policy []byte) {
		reported++
		got = append([]byte(nil), policy...)
	}
	t.Cleanup(func() { noteUEPolicyCommand = restore })

	ue := relayingUe(t)

	policy := []byte{0x11, 0x22, 0x33}
	SendDLNASTransport(ue, models.ACCESSTYPE__3_GPP_ACCESS,
		nasMessage.PayloadContainerTypeUEPolicy, policy, 0, 0, nil, 0)

	if reported != 1 {
		t.Fatalf("a downlink UE policy container was noted %d times, want 1: the COMMAND a "+
			"record carries was never held", reported)
	}
	if !bytes.Equal(got, policy) {
		t.Errorf("the hook was handed %v, want the container that was sent %v", got, policy)
	}
}

// TestADownlinkContainerOfAnotherKindIsNotNoted keeps the gate: the record is about UE
// policy, and holding an SM or SMS container as one would report an exchange that did not
// happen.
func TestADownlinkContainerOfAnotherKindIsNotNoted(t *testing.T) {
	reported := 0

	restore := noteUEPolicyCommand
	noteUEPolicyCommand = func(*context.AmfUe, []byte) { reported++ }
	t.Cleanup(func() { noteUEPolicyCommand = restore })

	ue := relayingUe(t)

	for _, kind := range []uint8{
		nasMessage.PayloadContainerTypeN1SMInfo,
		nasMessage.PayloadContainerTypeSMS,
		nasMessage.PayloadContainerTypeLPP,
	} {
		SendDLNASTransport(ue, models.ACCESSTYPE__3_GPP_ACCESS, kind, []byte{0x01}, 0, 0, nil, 0)
	}

	if reported != 0 {
		t.Errorf("%d containers of other kinds were noted as UE policy COMMANDs", reported)
	}
}

// TestTheBuilderIsWhereTheUEPolicyHookLives pins the seam, not the caller.
//
// SendDLNASTransport is not the seam every downlink UE-policy container crosses:
// producer/n1n2message.go builds the message with this builder and hands it to NGAP itself,
// never calling that function. It is also the only caller of this builder that passes a
// *variable* container type — every other one hard-codes N1SMInfo — so it is precisely the
// caller that can carry a policy.
//
// So the assertion is about the builder. A relay path that builds a DL NAS Transport carrying a
// policy container is noted whether or not it goes on to use SendDLNASTransport, which is what
// makes a path added later covered without anybody remembering to hook it.
func TestTheBuilderIsWhereTheUEPolicyHookLives(t *testing.T) {
	ue := relayingUe(t)

	var reported [][]byte

	restore := noteUEPolicyCommand
	noteUEPolicyCommand = func(_ *context.AmfUe, pdu []byte) {
		reported = append(reported, pdu)
	}

	t.Cleanup(func() { noteUEPolicyCommand = restore })

	policy := []byte{0x2e, 0x01, 0x03, 0x04, 0x05}

	// Built directly, as the N1N2 relay does — no SendDLNASTransport anywhere in this call.
	if _, err := BuildDLNASTransport(ue.AmfUe, models.ACCESSTYPE__3_GPP_ACCESS,
		nasMessage.PayloadContainerTypeUEPolicy, policy, 1, nil, nil, 0); err != nil {
		t.Fatalf("BuildDLNASTransport: %v", err)
	}

	if len(reported) != 1 {
		t.Fatalf("a downlink UE policy container built outside SendDLNASTransport was noted %d "+
			"times, want 1. The N1N2 relay path builds and sends the message itself, so a hook "+
			"on the sender never sees it, and the COMPLETE that follows has no COMMAND to "+
			"report", len(reported))
	}

	if string(reported[0]) != string(policy) {
		t.Errorf("the hook was handed %x, want the container that was transferred %x",
			reported[0], policy)
	}

	// And a container of another kind still produces nothing, so the gate did not widen.
	reported = nil

	if _, err := BuildDLNASTransport(ue.AmfUe, models.ACCESSTYPE__3_GPP_ACCESS,
		nasMessage.PayloadContainerTypeN1SMInfo, policy, 1, nil, nil, 0); err != nil {
		t.Fatalf("BuildDLNASTransport: %v", err)
	}

	if len(reported) != 0 {
		t.Errorf("an SM container was noted %d times as a UE policy; the gate must stay on the "+
			"container type", len(reported))
	}
}
