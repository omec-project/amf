// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	"bytes"
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"go.uber.org/zap"
)

// TestAnUplinkUEPolicyContainerReachesTheHook drives the transport handler, not the hook: the
// UE's MANAGE UE POLICY COMPLETE arrives in an UL NAS TRANSPORT, and that is the only place
// the record can be triggered. Without this test, removing the call leaves every test green
// while no UE policy record can ever be produced.
func TestAnUplinkUEPolicyContainerReachesTheHook(t *testing.T) {
	var got [][]byte
	restore := reportUEPolicyComplete
	reportUEPolicyComplete = func(_ *context.AmfUe, policy []byte) {
		got = append(got, append([]byte(nil), policy...))
	}
	t.Cleanup(func() { reportUEPolicyComplete = restore })

	// A UE with no N1 message subscription: the notify to the PCF sends nothing, so the test
	// needs no PCF.
	ue := &context.AmfUe{GmmLog: zap.NewNop().Sugar(), NASLog: zap.NewNop().Sugar()}

	complete := []byte{0x85, 0x02} // PTI, MANAGE UE POLICY COMPLETE
	ul := nasMessage.NewULNASTransport(0)
	ul.SetPayloadContainerType(nasMessage.PayloadContainerTypeUEPolicy)
	ul.PayloadContainer.SetLen(uint16(len(complete)))
	ul.SetPayloadContainerContents(complete)

	if err := HandleULNASTransport(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS, ul); err != nil {
		t.Fatalf("HandleULNASTransport: %v", err)
	}

	if len(got) != 1 || !bytes.Equal(got[0], complete) {
		t.Fatalf("the hook received %v, want the one container the UE sent %v", got, complete)
	}
}

// TestAnUplinkContainerOfAnotherKindDoesNotReachTheHook keeps the gate: only a UE policy
// container can carry a COMPLETE.
func TestAnUplinkContainerOfAnotherKindDoesNotReachTheHook(t *testing.T) {
	called := 0
	restore := reportUEPolicyComplete
	reportUEPolicyComplete = func(*context.AmfUe, []byte) { called++ }
	t.Cleanup(func() { reportUEPolicyComplete = restore })

	ue := &context.AmfUe{GmmLog: zap.NewNop().Sugar(), NASLog: zap.NewNop().Sugar()}
	for _, kind := range []uint8{nasMessage.PayloadContainerTypeSMS, nasMessage.PayloadContainerTypeLPP} {
		ul := nasMessage.NewULNASTransport(0)
		ul.SetPayloadContainerType(kind)
		ul.PayloadContainer.SetLen(2)
		ul.SetPayloadContainerContents([]byte{0x85, 0x02})
		//nolint:errcheck // these container types are refused; the assertion is on the hook
		_ = HandleULNASTransport(ctxt.Background(), ue, models.ACCESSTYPE__3_GPP_ACCESS, ul)
	}

	if called != 0 {
		t.Errorf("%d containers of other kinds reached the UE policy hook", called)
	}
}
