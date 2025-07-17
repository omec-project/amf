// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nas

import (
	ctxt "context"
	"errors"
	"fmt"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/gmm"
	"github.com/omec-project/nas"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/fsm"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("amf/nas")

func Dispatch(ctx ctxt.Context, ue *context.AmfUe, accessType models.AccessType, procedureCode int64, msg *nas.Message) error {
	if msg.GmmMessage == nil {
		return errors.New("gmm message is nil")
	}

	if msg.GsmMessage != nil {
		return errors.New("GSM Message should include in GMM Message")
	}

	if ue.State[accessType] == nil {
		return fmt.Errorf("UE State is empty (accessType=%q). Can't send GSM Message", accessType)
	}

	msgTypeName := nas.MessageName(msg.GmmHeader.GetMessageType())
	spanName := fmt.Sprintf("AMF NAS %s", msgTypeName)

	_, span := tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String("nas.accessType", string(accessType)),
			attribute.Int64("nas.procedureCode", procedureCode),
			attribute.String("nas.messageType", msgTypeName),
		),
	)
	defer span.End()

	return gmm.GmmFSM.SendEvent(ctx, ue.State[accessType], gmm.GmmMessageEvent, fsm.ArgsType{
		gmm.ArgAmfUe:         ue,
		gmm.ArgAccessType:    accessType,
		gmm.ArgNASMessage:    msg.GmmMessage,
		gmm.ArgProcedureCode: procedureCode,
	})
}
