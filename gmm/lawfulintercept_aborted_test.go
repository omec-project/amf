// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	ctxt "context"
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/fsm"
	"go.uber.org/zap"
)

func procedureUE(state fsm.StateType) *context.AmfUe {
	return &context.AmfUe{
		GmmLog: zap.NewNop().Sugar(),
		RanUe:  map[models.AccessType]*context.RanUe{},
		State: map[models.AccessType]*fsm.State{
			models.ACCESSTYPE__3_GPP_ACCESS:    fsm.NewState(state),
			models.ACCESSTYPE_NON_3_GPP_ACCESS: fsm.NewState(context.Deregistered),
		},
	}
}

// TestARegistrationProcedureRecordsWhereItBegan: every registration procedure starts with
// StartAuthEvent, from Registered or from Deregistered, and the UE records which, so a
// procedure that fails knows whether it ended a registration.
func TestARegistrationProcedureRecordsWhereItBegan(t *testing.T) {
	const access = models.ACCESSTYPE__3_GPP_ACCESS
	for _, c := range []struct {
		from     fsm.StateType
		callback func(ctxt.Context, *fsm.State, fsm.EventType, fsm.ArgsType)
		want     bool
	}{
		{context.Registered, Registered, true},
		{context.Deregistered, DeRegistered, false},
	} {
		ue := procedureUE(c.from)
		ue.SetRegisteredAtProcedureStart(access, !c.want)
		c.callback(ctxt.Background(), ue.State[access], StartAuthEvent,
			fsm.ArgsType{ArgAmfUe: ue, ArgAccessType: access})
		if got := ue.RegisteredAtProcedureStart(access); got != c.want {
			t.Errorf("a procedure starting from %s records %v, want %v", c.from, got, c.want)
		}
	}
}

// abortedCall is one call of the reportAbortedRegistration hook.
type abortedCall struct {
	ue      *context.AmfUe
	access  models.AccessType
	removed bool
}

// observeAbortedRegistrations replaces the hook for the test and records its calls. What
// lawfulintercept.ReportAbortedRegistration delivers for each argument set — an
// AMFDeregistration, network-initiated, for the access, and the deassociation — is asserted in
// that package, where interception can be activated.
func observeAbortedRegistrations(t *testing.T) *[]abortedCall {
	t.Helper()
	calls := &[]abortedCall{}
	original := reportAbortedRegistration
	reportAbortedRegistration = func(ue *context.AmfUe, access models.AccessType, removed bool) {
		*calls = append(*calls, abortedCall{ue, access, removed})
	}
	t.Cleanup(func() { reportAbortedRegistration = original })
	return calls
}

// TestEveryFailedProcedureAccountsForTheRegistrationItEnds: each event that drops a
// registration procedure to Deregistered hands that procedure, its access and the UE to
// lawfulintercept.ReportAbortedRegistration once, before the state is written, and without a
// removal: the UE context stays.
func TestEveryFailedProcedureAccountsForTheRegistrationItEnds(t *testing.T) {
	disableKafkaForRelayTest(t)
	const access = models.ACCESSTYPE__3_GPP_ACCESS
	for _, c := range []struct {
		state    fsm.StateType
		callback func(ctxt.Context, *fsm.State, fsm.EventType, fsm.ArgsType)
		event    fsm.EventType
	}{
		{context.Authentication, Authentication, AuthFailEvent},
		{context.Authentication, Authentication, AuthErrorEvent},
		{context.SecurityMode, SecurityMode, SecurityModeFailEvent},
		{context.SecurityMode, SecurityMode, SecurityModeAbortEvent},
		{context.ContextSetup, ContextSetup, ContextSetupFailEvent},
	} {
		calls := observeAbortedRegistrations(t)
		ue := procedureUE(c.state)
		ue.T3560 = context.NewTimer(time.Hour, 0, func(int32) {}, func() {})
		var stateAtReport fsm.StateType
		inner := reportAbortedRegistration
		reportAbortedRegistration = func(u *context.AmfUe, a models.AccessType, removed bool) {
			stateAtReport = u.State[a].Current()
			inner(u, a, removed)
		}

		c.callback(ctxt.Background(), ue.State[access], c.event,
			fsm.ArgsType{ArgAmfUe: ue, ArgAccessType: access})

		want := []abortedCall{{ue, access, false}}
		if len(*calls) != 1 || (*calls)[0] != want[0] {
			t.Errorf("%s in %s reported %+v, want %+v", c.event, c.state, *calls, want)
		}
		if stateAtReport != c.state {
			t.Errorf("%s in %s reported with the access already %s", c.event, c.state, stateAtReport)
		}
		if ue.T3560 != nil {
			ue.T3560.Stop()
		}
	}
}

// TestARejectThatRemovesTheUEAccountsForItsRegistrations: a registration rejected for
// congestion or a missing subscription removes the UE context, and every access with it, so
// the procedure is handed to lawfulintercept.ReportAbortedRegistration as a removal.
func TestARejectThatRemovesTheUEAccountsForItsRegistrations(t *testing.T) {
	disableKafkaForRelayTest(t)
	calls := observeAbortedRegistrations(t)
	originalReject := sendRegistrationRejectForRegistration
	sendRegistrationRejectForRegistration = func(*context.RanUe, uint8, string) {}
	t.Cleanup(func() { sendRegistrationRejectForRegistration = originalReject })

	const access = models.ACCESSTYPE__3_GPP_ACCESS
	ue := procedureUE(context.ContextSetup)
	rejectAndTearDownForRegistration(ue, nil, access, nasMessage.Cause5GMMCongestion)

	if want := (abortedCall{ue, access, true}); len(*calls) != 1 || (*calls)[0] != want {
		t.Errorf("the reject reported %+v, want %+v", *calls, want)
	}
}

// TestAContextSetupFailureStopsT3550: the failure ends the registration procedure, so the
// REGISTRATION ACCEPT T3550 guards is no longer awaited. Left running, its last expiry would
// register the UE the failure has just deregistered.
func TestAContextSetupFailureStopsT3550(t *testing.T) {
	disableKafkaForRelayTest(t)
	const access = models.ACCESSTYPE__3_GPP_ACCESS
	ue := procedureUE(context.ContextSetup)
	ue.T3550 = context.NewTimer(time.Hour, 0, func(int32) {}, func() {})

	ContextSetup(ctxt.Background(), ue.State[access], ContextSetupFailEvent,
		fsm.ArgsType{ArgAmfUe: ue, ArgAccessType: access})

	if ue.T3550 != nil {
		ue.T3550.Stop()
		t.Error("T3550 still runs after the context setup failure ended the procedure")
	}
}

// TestANetworkDeregistrationThatRemovesTheUEAccountsForTheOtherAccess: a network-initiated
// deregistration of an unconnected UE removes the UE context, and a registration on the other
// access with it, so the removal is handed to lawfulintercept.ReportAbortedRegistration, which
// reports that access.
func TestANetworkDeregistrationThatRemovesTheUEAccountsForTheOtherAccess(t *testing.T) {
	disableKafkaForRelayTest(t)
	calls := observeAbortedRegistrations(t)
	const access = models.ACCESSTYPE__3_GPP_ACCESS
	ue := procedureUE(context.Registered)
	ue.State[models.ACCESSTYPE_NON_3_GPP_ACCESS] = fsm.NewState(context.Registered)

	if err := NetworkInitiatedDeregistrationProcedure(ctxt.Background(), ue, access); err != nil {
		t.Fatalf("NetworkInitiatedDeregistrationProcedure: %v", err)
	}

	if want := (abortedCall{ue, access, true}); len(*calls) != 1 || (*calls)[0] != want {
		t.Errorf("the removal reported %+v, want %+v", *calls, want)
	}
}
