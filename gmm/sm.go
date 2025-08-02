// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package gmm

import (
	ctxt "context"
	"fmt"

	"github.com/omec-project/amf/context"
	gmm_message "github.com/omec-project/amf/gmm/message"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/nas"
	"github.com/omec-project/nas/nasMessage"
	"github.com/omec-project/nas/security"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/fsm"
)

func DeRegistered(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	switch event {
	case fsm.EntryEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		accessType := args[ArgAccessType].(models.AccessType)
		amfUe.ClearRegistrationRequestData(accessType)
		amfUe.GmmLog.Debugln("EntryEvent at GMM State[DeRegistered]")
	case GmmMessageEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		procedureCode := args[ArgProcedureCode].(int64)
		gmmMessage := args[ArgNASMessage].(*nas.GmmMessage)
		accessType := args[ArgAccessType].(models.AccessType)
		amfUe.GmmLog.Debugln("GmmMessageEvent at GMM State[DeRegistered]")
		switch gmmMessage.GetMessageType() {
		case nas.MsgTypeRegistrationRequest:
			if err := HandleRegistrationRequest(ctx, amfUe, accessType, procedureCode, gmmMessage.RegistrationRequest); err != nil {
				logger.GmmLog.Errorln(err)
			} else {
				if err := GmmFSM.SendEvent(ctx, state, StartAuthEvent, fsm.ArgsType{
					ArgAmfUe:         amfUe,
					ArgAccessType:    accessType,
					ArgProcedureCode: procedureCode,
				}); err != nil {
					logger.GmmLog.Errorln(err)
				}
			}
		case nas.MsgTypeServiceRequest:
			if err := HandleServiceRequest(ctx, amfUe, accessType, gmmMessage.ServiceRequest); err != nil {
				logger.GmmLog.Errorln(err)
			}
		default:
			amfUe.GmmLog.Errorf("state mismatch: receieve gmm message[message type 0x%0x] at %s state",
				gmmMessage.GetMessageType(), state.Current())
		}
	case NwInitiatedDeregistrationEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		accessType := args[ArgAccessType].(models.AccessType)
		if err := NetworkInitiatedDeregistrationProcedure(ctx, amfUe, accessType); err != nil {
			logger.GmmLog.Errorln(err)
		}
	case StartAuthEvent:
		logger.GmmLog.Debugln(event)
	case fsm.ExitEvent:
		logger.GmmLog.Debugln(event)
	default:
		logger.GmmLog.Errorf("Unknown event [%+v]", event)
	}
}

func Registered(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	switch event {
	case fsm.EntryEvent:
		// clear stored registration request data for this registration
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		accessType := args[ArgAccessType].(models.AccessType)
		amfUe.ClearRegistrationRequestData(accessType)
		amfUe.GmmLog.Debugln("EntryEvent at GMM State[Registered]")
		// store context in DB. Registration procedure is complete.
		amfUe.PublishUeCtxtInfo()
		context.StoreContextInDB(amfUe)
	case GmmMessageEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		procedureCode := args[ArgProcedureCode].(int64)
		gmmMessage := args[ArgNASMessage].(*nas.GmmMessage)
		accessType := args[ArgAccessType].(models.AccessType)
		amfUe.GmmLog.Debugln("GmmMessageEvent at GMM State[Registered]")
		switch gmmMessage.GetMessageType() {
		// Mobility Registration update / Periodic Registration update
		case nas.MsgTypeRegistrationRequest:
			if err := HandleRegistrationRequest(ctx, amfUe, accessType, procedureCode, gmmMessage.RegistrationRequest); err != nil {
				logger.GmmLog.Errorln(err)
			} else {
				if err := GmmFSM.SendEvent(ctx, state, StartAuthEvent, fsm.ArgsType{
					ArgAmfUe:         amfUe,
					ArgAccessType:    accessType,
					ArgProcedureCode: procedureCode,
				}); err != nil {
					logger.GmmLog.Errorln(err)
				}
			}
		case nas.MsgTypeULNASTransport:
			if err := HandleULNASTransport(ctx, amfUe, accessType, gmmMessage.ULNASTransport); err != nil {
				logger.GmmLog.Errorln(err)
			}
		case nas.MsgTypeConfigurationUpdateComplete:
			if err := HandleConfigurationUpdateComplete(amfUe, gmmMessage.ConfigurationUpdateComplete); err != nil {
				logger.GmmLog.Errorln(err)
			}
		case nas.MsgTypeServiceRequest:
			if err := HandleServiceRequest(ctx, amfUe, accessType, gmmMessage.ServiceRequest); err != nil {
				logger.GmmLog.Errorln(err)
			}
		case nas.MsgTypeNotificationResponse:
			if err := HandleNotificationResponse(amfUe, gmmMessage.NotificationResponse); err != nil {
				logger.GmmLog.Errorln(err)
			}
		case nas.MsgTypeDeregistrationRequestUEOriginatingDeregistration:
			if err := GmmFSM.SendEvent(ctx, state, InitDeregistrationEvent, fsm.ArgsType{
				ArgAmfUe:      amfUe,
				ArgAccessType: accessType,
				ArgNASMessage: gmmMessage,
			}); err != nil {
				logger.GmmLog.Errorln(err)
			}
		case nas.MsgTypeStatus5GMM:
			if err := HandleStatus5GMM(amfUe, accessType, gmmMessage.Status5GMM); err != nil {
				logger.GmmLog.Errorln(err)
			}
		default:
			amfUe.GmmLog.Errorf("state mismatch: receieve gmm message[message type 0x%0x] at %s state",
				gmmMessage.GetMessageType(), state.Current())
		}
	case StartAuthEvent:
		logger.GmmLog.Debugln(event)
	case InitDeregistrationEvent:
		logger.GmmLog.Debugln(event)
	case NwInitiatedDeregistrationEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		accessType := args[ArgAccessType].(models.AccessType)
		if err := NetworkInitiatedDeregistrationProcedure(ctx, amfUe, accessType); err != nil {
			logger.GmmLog.Errorln(err)
		}
	/*TODO */
	case SliceInfoAddEvent:
	case SliceInfoDeleteEvent:
	case fsm.ExitEvent:
		logger.GmmLog.Debugln(event)
	default:
		logger.GmmLog.Errorf("Unknown event [%+v]", event)
	}
}

func Authentication(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	var amfUe *context.AmfUe
	switch event {
	case fsm.EntryEvent:
		amfUe = args[ArgAmfUe].(*context.AmfUe)
		amfUe.GmmLog = amfUe.GmmLog.With(logger.FieldSuci, amfUe.Suci)
		amfUe.TxLog = amfUe.TxLog.With(logger.FieldSuci, amfUe.Suci)
		amfUe.GmmLog.Debugln("entryEvent at GMM State[Authentication]")
		amfUe.PublishUeCtxtInfo()
		fallthrough
	case AuthRestartEvent:
		amfUe = args[ArgAmfUe].(*context.AmfUe)
		accessType := args[ArgAccessType].(models.AccessType)
		amfUe.GmmLog.Debugln("authRestartEvent at GMM State[Authentication]")

		pass, err := AuthenticationProcedure(ctx, amfUe, accessType)
		if err != nil {
			if err := GmmFSM.SendEvent(ctx, state, AuthErrorEvent, fsm.ArgsType{
				ArgAmfUe:      amfUe,
				ArgAccessType: accessType,
			}); err != nil {
				logger.GmmLog.Errorln(err)
			}
		}
		if pass {
			if err := GmmFSM.SendEvent(ctx, state, AuthSuccessEvent, fsm.ArgsType{
				ArgAmfUe:      amfUe,
				ArgAccessType: accessType,
			}); err != nil {
				logger.GmmLog.Errorln(err)
			}
		}
	case GmmMessageEvent:
		amfUe = args[ArgAmfUe].(*context.AmfUe)
		gmmMessage := args[ArgNASMessage].(*nas.GmmMessage)
		accessType := args[ArgAccessType].(models.AccessType)
		amfUe.GmmLog.Debugln("GmmMessageEvent at GMM State[Authentication]")

		switch gmmMessage.GetMessageType() {
		case nas.MsgTypeIdentityResponse:
			if err := HandleIdentityResponse(amfUe, gmmMessage.IdentityResponse); err != nil {
				logger.GmmLog.Errorln(err)
			}
			err := GmmFSM.SendEvent(ctx, state, AuthRestartEvent, fsm.ArgsType{ArgAmfUe: amfUe, ArgAccessType: accessType})
			if err != nil {
				logger.GmmLog.Errorln(err)
			}
		case nas.MsgTypeAuthenticationResponse:
			if err := HandleAuthenticationResponse(ctx, amfUe, accessType, gmmMessage.AuthenticationResponse); err != nil {
				logger.GmmLog.Errorln(err)
			}
			amfUe.PublishUeCtxtInfo()
		case nas.MsgTypeAuthenticationFailure:
			if err := HandleAuthenticationFailure(ctx, amfUe, accessType, gmmMessage.AuthenticationFailure); err != nil {
				logger.GmmLog.Errorln(err)
			}
		case nas.MsgTypeStatus5GMM:
			if err := HandleStatus5GMM(amfUe, accessType, gmmMessage.Status5GMM); err != nil {
				logger.GmmLog.Errorln(err)
			}
		default:
			logger.GmmLog.Errorf("UE state mismatch: receieve gmm message[message type 0x%0x] at %s state",
				gmmMessage.GetMessageType(), state.Current())
			// called SendEvent() to move to deregistered state if state mismatch occurs
			err := GmmFSM.SendEvent(ctx, state, AuthFailEvent, fsm.ArgsType{
				ArgAmfUe:      amfUe,
				ArgAccessType: accessType,
			})
			if err != nil {
				logger.GmmLog.Errorln(err)
			} else {
				amfUe.GmmLog.Info("state reset to Deregistered")
			}
		}
	case AuthSuccessEvent:
		logger.GmmLog.Debugln(event)
	case AuthFailEvent:
		logger.GmmLog.Debugln(event)
		logger.GmmLog.Warnln("reject authentication")
	case AuthErrorEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		accessType := args[ArgAccessType].(models.AccessType)
		logger.GmmLog.Debugln(event)
		if err := HandleAuthenticationError(amfUe, accessType); err != nil {
			logger.GmmLog.Errorln(err)
		}
	case NwInitiatedDeregistrationEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		accessType := args[ArgAccessType].(models.AccessType)
		if err := NetworkInitiatedDeregistrationProcedure(ctx, amfUe, accessType); err != nil {
			logger.GmmLog.Errorln(err)
		}
	case fsm.ExitEvent:
		// clear authentication related data at exit
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		amfUe.GmmLog.Debugln(event)
		amfUe.AuthenticationCtx = nil
		amfUe.AuthFailureCauseSynchFailureTimes = 0
	default:
		logger.GmmLog.Errorf("Unknown event [%+v]", event)
	}
}

func SecurityMode(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	switch event {
	case fsm.EntryEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		accessType := args[ArgAccessType].(models.AccessType)
		// set log information
		amfUe.NASLog = amfUe.NASLog.With(logger.FieldSupi, fmt.Sprintf("SUPI:%s", amfUe.Supi))
		amfUe.TxLog = amfUe.NASLog.With(logger.FieldSupi, fmt.Sprintf("SUPI:%s", amfUe.Supi))
		amfUe.GmmLog = amfUe.GmmLog.With(logger.FieldSupi, fmt.Sprintf("SUPI:%s", amfUe.Supi))
		amfUe.ProducerLog = logger.ProducerLog.With(logger.FieldSupi, fmt.Sprintf("SUPI:%s", amfUe.Supi))
		amfUe.PublishUeCtxtInfo()
		amfUe.GmmLog.Debugln("EntryEvent at GMM State[SecurityMode]")
		if amfUe.SecurityContextIsValid() {
			amfUe.GmmLog.Debugln("UE has a valid security context - skip security mode control procedure")
			if err := GmmFSM.SendEvent(ctx, state, SecurityModeSuccessEvent, fsm.ArgsType{
				ArgAmfUe:      amfUe,
				ArgAccessType: accessType,
				ArgNASMessage: amfUe.RegistrationRequest,
			}); err != nil {
				logger.GmmLog.Errorln(err)
			}
		} else {
			eapSuccess := args[ArgEAPSuccess].(bool)
			eapMessage := args[ArgEAPMessage].(string)
			// Select enc/int algorithm based on ue security capability & amf's policy,
			amfSelf := context.AMF_Self()
			amfUe.SelectSecurityAlg(amfSelf.SecurityAlgorithm.IntegrityOrder, amfSelf.SecurityAlgorithm.CipheringOrder)
			// Generate KnasEnc, KnasInt
			amfUe.DerivateAlgKey()
			if amfUe.CipheringAlg == security.AlgCiphering128NEA0 && amfUe.IntegrityAlg == security.AlgIntegrity128NIA0 {
				err := GmmFSM.SendEvent(ctx, state, SecuritySkipEvent, fsm.ArgsType{
					ArgAmfUe:      amfUe,
					ArgAccessType: accessType,
					ArgNASMessage: amfUe.RegistrationRequest,
				})
				if err != nil {
					logger.GmmLog.Errorln(err)
				}
			} else {
				gmm_message.SendSecurityModeCommand(amfUe.RanUe[accessType], accessType, eapSuccess, eapMessage)
			}
		}
	case GmmMessageEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		procedureCode := args[ArgProcedureCode].(int64)
		gmmMessage := args[ArgNASMessage].(*nas.GmmMessage)
		accessType := args[ArgAccessType].(models.AccessType)
		amfUe.GmmLog.Debugln("GmmMessageEvent to GMM State[SecurityMode]")
		switch gmmMessage.GetMessageType() {
		case nas.MsgTypeSecurityModeComplete:
			if err := HandleSecurityModeComplete(ctx, amfUe, accessType, procedureCode, gmmMessage.SecurityModeComplete); err != nil {
				logger.GmmLog.Errorln(err)
			}
		case nas.MsgTypeSecurityModeReject:
			if err := HandleSecurityModeReject(amfUe, accessType, gmmMessage.SecurityModeReject); err != nil {
				logger.GmmLog.Errorln(err)
			}
			err := GmmFSM.SendEvent(ctx, state, SecurityModeFailEvent, fsm.ArgsType{
				ArgAmfUe:      amfUe,
				ArgAccessType: accessType,
			})
			if err != nil {
				logger.GmmLog.Errorln(err)
			}
		case nas.MsgTypeRegistrationRequest:
			// Sending AbortEvent to ongoing procedure
			err := GmmFSM.SendEvent(ctx, state, SecurityModeAbortEvent, fsm.ArgsType{
				ArgAmfUe:      amfUe,
				ArgAccessType: accessType,
			})
			if err != nil {
				logger.GmmLog.Errorln(err)
			}

			err = GmmFSM.SendEvent(ctx, state, GmmMessageEvent, fsm.ArgsType{
				ArgAmfUe:         amfUe,
				ArgAccessType:    accessType,
				ArgNASMessage:    gmmMessage,
				ArgProcedureCode: procedureCode,
			})
			if err != nil {
				logger.GmmLog.Errorln(err)
			}

		case nas.MsgTypeStatus5GMM:
			if err := HandleStatus5GMM(amfUe, accessType, gmmMessage.Status5GMM); err != nil {
				logger.GmmLog.Errorln(err)
			}
		default:
			amfUe.GmmLog.Errorf("state mismatch: receieve gmm message[message type 0x%0x] at %s state",
				gmmMessage.GetMessageType(), state.Current())
			// called SendEvent() to move to deregistered state if state mismatch occurs
			err := GmmFSM.SendEvent(ctx, state, SecurityModeFailEvent, fsm.ArgsType{
				ArgAmfUe:      amfUe,
				ArgAccessType: accessType,
			})
			if err != nil {
				logger.GmmLog.Errorln(err)
			} else {
				amfUe.GmmLog.Info("state reset to Deregistered")
			}
		}
	case SecurityModeAbortEvent:
		logger.GmmLog.Debugln(event)
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		// stopping security mode command timer
		amfUe.SecurityContextAvailable = false
		amfUe.T3560.Stop()
		amfUe.T3560 = nil
	case NwInitiatedDeregistrationEvent:
		logger.GmmLog.Debugln(event)
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		accessType := args[ArgAccessType].(models.AccessType)
		amfUe.T3560.Stop()
		amfUe.T3560 = nil
		if err := NetworkInitiatedDeregistrationProcedure(ctx, amfUe, accessType); err != nil {
			logger.GmmLog.Errorln(err)
		}
	case SecurityModeSuccessEvent:
		logger.GmmLog.Debugln(event)
	case SecurityModeFailEvent:
		logger.GmmLog.Debugln(event)
	case fsm.ExitEvent:
		logger.GmmLog.Debugln(event)
		return
	default:
		logger.GmmLog.Errorf("unknown event [%+v]", event)
	}
}

func ContextSetup(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	switch event {
	case fsm.EntryEvent:
		amfUe, ok := args[ArgAmfUe].(*context.AmfUe)
		if !ok {
			logger.GmmLog.Errorln("invalid type assertion for ArgAmfUe")
			return
		}
		gmmMessage := args[ArgNASMessage]
		accessType, ok := args[ArgAccessType].(models.AccessType)
		if !ok {
			logger.GmmLog.Errorln("invalid type assertion for ArgAccessType")
			return
		}
		amfUe.GmmLog.Debugln("EntryEvent at GMM State[ContextSetup]")
		amfUe.PublishUeCtxtInfo()
		switch message := gmmMessage.(type) {
		case *nasMessage.RegistrationRequest:
			amfUe.RegistrationRequest = message
			switch amfUe.RegistrationType5GS {
			case nasMessage.RegistrationType5GSInitialRegistration:
				if err := HandleInitialRegistration(ctx, amfUe, accessType); err != nil {
					logger.GmmLog.Errorln(err)
				}
			case nasMessage.RegistrationType5GSMobilityRegistrationUpdating:
				fallthrough
			case nasMessage.RegistrationType5GSPeriodicRegistrationUpdating:
				if err := HandleMobilityAndPeriodicRegistrationUpdating(ctx, amfUe, accessType); err != nil {
					logger.GmmLog.Errorln(err)
				}
			}
		case *nasMessage.ServiceRequest:
			if err := HandleServiceRequest(ctx, amfUe, accessType, message); err != nil {
				logger.GmmLog.Errorln(err)
			}
		default:
			logger.GmmLog.Errorf("UE state mismatch: receieve wrong gmm message")
		}
	case GmmMessageEvent:
		amfUe, ok := args[ArgAmfUe].(*context.AmfUe)
		if !ok {
			logger.GmmLog.Errorln("invalid type assertion for ArgAmfUe")
			return
		}
		gmmMessage, ok := args[ArgNASMessage].(*nas.GmmMessage)
		if !ok {
			logger.GmmLog.Errorln("invalid type assertion for ArgNASMessage")
			return
		}
		accessType, ok := args[ArgAccessType].(models.AccessType)
		if !ok {
			logger.GmmLog.Errorln("invalid type assertion for ArgAccessType")
			return
		}
		amfUe.GmmLog.Debugln("GmmMessageEvent at GMM State[ContextSetup]")
		switch gmmMessage.GetMessageType() {
		case nas.MsgTypeIdentityResponse:
			if err := HandleIdentityResponse(amfUe, gmmMessage.IdentityResponse); err != nil {
				logger.GmmLog.Errorln(err)
			}
			switch amfUe.RegistrationType5GS {
			case nasMessage.RegistrationType5GSInitialRegistration:
				if err := HandleInitialRegistration(ctx, amfUe, accessType); err != nil {
					logger.GmmLog.Errorln(err)
					err = GmmFSM.SendEvent(ctx, state, ContextSetupFailEvent, fsm.ArgsType{
						ArgAmfUe:      amfUe,
						ArgAccessType: accessType,
					})
					if err != nil {
						logger.GmmLog.Errorln(err)
					}
				}
			case nasMessage.RegistrationType5GSMobilityRegistrationUpdating:
				fallthrough
			case nasMessage.RegistrationType5GSPeriodicRegistrationUpdating:
				if err := HandleMobilityAndPeriodicRegistrationUpdating(ctx, amfUe, accessType); err != nil {
					logger.GmmLog.Errorln(err)
					err = GmmFSM.SendEvent(ctx, state, ContextSetupFailEvent, fsm.ArgsType{
						ArgAmfUe:      amfUe,
						ArgAccessType: accessType,
					})
					if err != nil {
						logger.GmmLog.Errorln(err)
					}
				}
			}
		case nas.MsgTypeRegistrationComplete:
			if err := HandleRegistrationComplete(ctx, amfUe, accessType, gmmMessage.RegistrationComplete); err != nil {
				logger.GmmLog.Errorln(err)
			}
		case nas.MsgTypeStatus5GMM:
			if err := HandleStatus5GMM(amfUe, accessType, gmmMessage.Status5GMM); err != nil {
				logger.GmmLog.Errorln(err)
			}
		default:
			amfUe.GmmLog.Errorf("state mismatch: receieve gmm message[message type 0x%0x] at %s state",
				gmmMessage.GetMessageType(), state.Current())
			msgType := gmmMessage.GetMessageType()
			if msgType == nas.MsgTypeRegistrationRequest {
				// called SendEvent() to move to deregistered state if state mismatch occurs
				err := GmmFSM.SendEvent(ctx, state, ContextSetupFailEvent, fsm.ArgsType{
					ArgAmfUe:      amfUe,
					ArgAccessType: accessType,
				})
				if err != nil {
					logger.GmmLog.Errorln(err)
				} else {
					amfUe.GmmLog.Info("state reset to Deregistered")
				}
			}
		}
	case ContextSetupSuccessEvent:
		logger.GmmLog.Debugln(event)
	case NwInitiatedDeregistrationEvent:
		logger.GmmLog.Debugln(event)
		amfUe, ok := args[ArgAmfUe].(*context.AmfUe)
		if !ok {
			logger.GmmLog.Errorln("invalid type assertion for ArgAmfUe")
			return
		}
		accessType, ok := args[ArgAccessType].(models.AccessType)
		if !ok {
			logger.GmmLog.Errorln("invalid type assertion for ArgAccessType")
			return
		}
		amfUe.T3550.Stop()
		amfUe.T3550 = nil
		amfUe.State[accessType].Set(context.Registered)
		if err := NetworkInitiatedDeregistrationProcedure(ctx, amfUe, accessType); err != nil {
			logger.GmmLog.Errorln(err)
		}
	case ContextSetupFailEvent:
		logger.GmmLog.Debugln(event)
	case fsm.ExitEvent:
		logger.GmmLog.Debugln(event)
	default:
		logger.GmmLog.Errorf("unknown event [%+v]", event)
	}
}

func DeregisteredInitiated(ctx ctxt.Context, state *fsm.State, event fsm.EventType, args fsm.ArgsType) {
	switch event {
	case fsm.EntryEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		if args[ArgNASMessage] != nil {
			gmmMessage := args[ArgNASMessage].(*nas.GmmMessage)
			if gmmMessage != nil {
				accessType := args[ArgAccessType].(models.AccessType)
				amfUe.GmmLog.Debugln("EntryEvent at GMM State[DeregisteredInitiated]")
				if err := HandleDeregistrationRequest(ctx, amfUe, accessType,
					gmmMessage.DeregistrationRequestUEOriginatingDeregistration); err != nil {
					logger.GmmLog.Errorln(err)
				}
			}
		}
		amfUe.PublishUeCtxtInfo()
	case GmmMessageEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		gmmMessage := args[ArgNASMessage].(*nas.GmmMessage)
		accessType := args[ArgAccessType].(models.AccessType)
		amfUe.GmmLog.Debugln("GmmMessageEvent at GMM State[DeregisteredInitiated]")
		switch gmmMessage.GetMessageType() {
		case nas.MsgTypeDeregistrationAcceptUETerminatedDeregistration:
			if err := HandleDeregistrationAccept(ctx, amfUe, accessType,
				gmmMessage.DeregistrationAcceptUETerminatedDeregistration); err != nil {
				logger.GmmLog.Errorln(err)
			}
		default:
			amfUe.GmmLog.Errorf("state mismatch: receieve gmm message[message type 0x%0x] at %s state",
				gmmMessage.GetMessageType(), state.Current())
			// called SendEvent() to move to deregistered state if state mismatch occurs
			err := GmmFSM.SendEvent(ctx, state, DeregistrationAcceptEvent, fsm.ArgsType{
				ArgAmfUe:      amfUe,
				ArgAccessType: accessType,
			})
			if err != nil {
				logger.GmmLog.Errorln(err)
			} else {
				amfUe.GmmLog.Info("state reset to Deregistered")
			}
		}
	case DeregistrationAcceptEvent:
		amfUe := args[ArgAmfUe].(*context.AmfUe)
		accessType := args[ArgAccessType].(models.AccessType)
		SetDeregisteredState(amfUe, util.AnTypeToNas(accessType))
		logger.GmmLog.Debugln(event)
	case fsm.ExitEvent:
		logger.GmmLog.Debugln(event)
	default:
		logger.GmmLog.Errorf("unknown event [%+v]", event)
	}
}

func SetDeregisteredState(amfUe *context.AmfUe, anType uint8) {
	amfUe.SubscriptionDataValid = false
	switch anType {
	case nasMessage.AccessType3GPP:
		amfUe.GmmLog.Warnln("UE accessType[3GPP] transfer to Deregistered state")
		amfUe.State[models.AccessType__3_GPP_ACCESS].Set(context.Deregistered)
	case nasMessage.AccessTypeNon3GPP:
		amfUe.GmmLog.Warnln("UE accessType[Non3GPP] transfer to Deregistered state")
		amfUe.State[models.AccessType_NON_3_GPP_ACCESS].Set(context.Deregistered)
	default:
		amfUe.GmmLog.Warnln("UE accessType[3GPP] transfer to Deregistered state")
		amfUe.State[models.AccessType__3_GPP_ACCESS].Set(context.Deregistered)
		amfUe.GmmLog.Warnln("UE accessType[Non3GPP] transfer to Deregistered state")
		amfUe.State[models.AccessType_NON_3_GPP_ACCESS].Set(context.Deregistered)
	}
}
