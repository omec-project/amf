// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nas

import (
	ctx "context"
	"errors"
	"fmt"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/gmm"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/nas"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/fsm"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("amf/nas")

func messageTypeName(code uint8) string {
	switch code {
	case 65:
		return "RegistrationRequest"
	case 66:
		return "RegistrationAccept"
	case 67:
		return "RegistrationComplete"
	case 68:
		return "RegistrationReject"
	case 69:
		return "DeregistrationRequestUEOriginatingDeregistration"
	case 70:
		return "DeregistrationAcceptUEOriginatingDeregistration"
	case 71:
		return "DeregistrationRequestUETerminatedDeregistration"
	case 72:
		return "DeregistrationAcceptUETerminatedDeregistration"
	case 76:
		return "ServiceRequest"
	case 77:
		return "ServiceReject"
	case 78:
		return "ServiceAccept"
	case 84:
		return "ConfigurationUpdateCommand"
	case 85:
		return "ConfigurationUpdateComplete"
	case 86:
		return "AuthenticationRequest"
	case 87:
		return "AuthenticationResponse"
	case 88:
		return "AuthenticationReject"
	case 89:
		return "AuthenticationFailure"
	case 90:
		return "AuthenticationResult"
	case 91:
		return "IdentityRequest"
	case 92:
		return "IdentityResponse"
	case 93:
		return "SecurityModeCommand"
	case 94:
		return "SecurityModeComplete"
	case 95:
		return "SecurityModeReject"
	case 100:
		return "Status5GMM"
	case 101:
		return "Notification"
	case 102:
		return "NotificationResponse"
	case 103:
		return "ULNASTransport"
	case 104:
		return "DLNASTransport"
	default:
		return fmt.Sprintf("Unknown message type: %d", code)
	}
}

func Dispatch(ctext ctx.Context, ue *context.AmfUe, accessType models.AccessType, procedureCode int64, msg *nas.Message) error {
	if msg.GmmMessage == nil {
		return errors.New("gmm message is nil")
	}

	if msg.GsmMessage != nil {
		return errors.New("GSM Message should include in GMM Message")
	}

	if ue.State[accessType] == nil {
		return fmt.Errorf("UE State is empty (accessType=%q). Can't send GSM Message", accessType)
	}

	msgTypeName := messageTypeName(msg.GmmHeader.GetMessageType())
	spanName := fmt.Sprintf("AMF NAS %s", msgTypeName)

	_, span := tracer.Start(ctext, spanName,
		trace.WithAttributes(
			attribute.String("nas.accessType", string(accessType)),
			attribute.Int64("nas.procedureCode", procedureCode),
			attribute.String("nas.messageType", msgTypeName),
		),
	)
	defer span.End()

	logger.AppLog.Warnf("created span for %s, accessType: %s, procedureCode: %d, messageType: %s",
		spanName, accessType, procedureCode, msgTypeName)

	return gmm.GmmFSM.SendEvent(ue.State[accessType], gmm.GmmMessageEvent, fsm.ArgsType{
		gmm.ArgAmfUe:         ue,
		gmm.ArgAccessType:    accessType,
		gmm.ArgNASMessage:    msg.GmmMessage,
		gmm.ArgProcedureCode: procedureCode,
	})
}
