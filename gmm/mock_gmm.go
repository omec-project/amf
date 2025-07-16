// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
//

package gmm

import (
	ctxt "context"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/util/fsm"
)

var (
	MockRegisteredCallCount            uint32 = 0
	MockDeregisteredInitiatedCallCount uint32 = 0
	MockContextSetupCallCount          uint32 = 0
	MockDeRegisteredCallCount          uint32 = 0
	MockSecurityModeCallCount          uint32 = 0
	MockAuthenticationCallCount        uint32 = 0
)

var mockCallbacks = fsm.Callbacks{
	context.Deregistered:            MockDeRegistered,
	context.Authentication:          MockAuthentication,
	context.SecurityMode:            MockSecurityMode,
	context.ContextSetup:            MockContextSetup,
	context.Registered:              MockRegistered,
	context.DeregistrationInitiated: MockDeregisteredInitiated,
}

func Mockinit() {
	if f, err := fsm.NewFSM(transitions, mockCallbacks); err != nil {
		logger.GmmLog.Errorf("Initialize Gmm FSM Error: %+v", err)
	} else {
		GmmFSM = f
	}
}

func MockDeRegistered(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	logger.GmmLog.Info("MockDeRegistered")
	MockDeRegisteredCallCount++
}

func MockAuthentication(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	logger.GmmLog.Info("MockAuthentication")
	MockAuthenticationCallCount++
}

func MockSecurityMode(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	logger.GmmLog.Info("MockSecurityMode")
	MockSecurityModeCallCount++
}

func MockContextSetup(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	logger.GmmLog.Info("MockContextSetup")
	MockContextSetupCallCount++
}

func MockRegistered(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	logger.GmmLog.Info(event)
	logger.GmmLog.Info("MockRegistered")
	MockRegisteredCallCount++
}

func MockDeregisteredInitiated(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	logger.GmmLog.Info("MockDeregisteredInitiated")
	MockDeregisteredInitiatedCallCount++

	amfUe := args[ArgAmfUe].(*context.AmfUe)
	amfUe.Remove()
}
