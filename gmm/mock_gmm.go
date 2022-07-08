package gmm

import (
	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/fsm"
)

var mockCallbacks = fsm.Callbacks{
	context.Deregistered:            DeRegistered,
	context.Authentication:          Authentication,
	context.SecurityMode:            SecurityMode,
	context.ContextSetup:            ContextSetup,
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

func MockRegistered(state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	logger.GmmLog.Info("MockRegistered")
}

func MockDeregisteredInitiated(state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	logger.GmmLog.Info("MockDeregisteredInitiated")
	amfUe := args[ArgAmfUe].(*context.AmfUe)
	amfUe.Remove()
}
