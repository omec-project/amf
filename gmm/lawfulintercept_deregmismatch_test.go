// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"github.com/omec-project/util/li/iri"
	"go.uber.org/zap"
)

// TestAnUnexpectedMessageEndsOnlyTheNetworksDeregistrationOnce: another message from the UE in
// DeregistrationInitiated ends a network-initiated deregistration, which is reported there. A
// UE-originating deregistration naming an access type HandleDeregistrationRequest does not act
// on also leaves the UE in that state, and was reported when the request arrived; ending it
// here reports nothing more.
func TestAnUnexpectedMessageEndsOnlyTheNetworksDeregistrationOnce(t *testing.T) {
	disableKafkaForRelayTest(t)
	const access = models.ACCESSTYPE__3_GPP_ACCESS
	for _, c := range []struct {
		name       string
		target     uint8
		wantDereg  int
		wantDeasso int
	}{
		{"network-initiated", nasMessage.AccessType3GPP, 1, 1},
		{"UE-originating, already reported", 0, 0, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			var dereg, deasso int
			restoreDereg, restoreDeasso := reportDeregistration, reportIdentifierDeassociation
			reportDeregistration = func(*context.AmfUe, bool, iri.AccessType, []byte) { dereg++ }
			reportIdentifierDeassociation = func(*context.AmfUe) { deasso++ }
			t.Cleanup(func() {
				reportDeregistration, reportIdentifierDeassociation = restoreDereg, restoreDeasso
			})

			ue := &context.AmfUe{
				GmmLog: zap.NewNop().Sugar(),
				RanUe:  map[models.AccessType]*context.RanUe{},
				State: map[models.AccessType]*fsm.State{
					access:                             fsm.NewState(context.DeregistrationInitiated),
					models.ACCESSTYPE_NON_3_GPP_ACCESS: fsm.NewState(context.Deregistered),
				},
				OnGoing: map[models.AccessType]*context.OnGoingProcedureWithPrio{
					access: {Procedure: context.OnGoingProcedureNothing},
				},
				DeregistrationTargetAccessType: c.target,
			}
			msg := nas.NewGmmMessage()
			msg.SetMessageType(nas.MsgTypeServiceRequest)

			DeregisteredInitiated(ctxt.Background(), ue.State[access], GmmMessageEvent, fsm.ArgsType{
				ArgAmfUe: ue, ArgAccessType: access, ArgNASMessage: msg,
			})

			if dereg != c.wantDereg || deasso != c.wantDeasso {
				t.Errorf("reported %d deregistrations and %d deassociations, want %d and %d",
					dereg, deasso, c.wantDereg, c.wantDeasso)
			}
		})
	}
}
