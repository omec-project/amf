// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package ngap

import (
	ctxt "context"
	"fmt"
	"net"
	"os"
	"reflect"

	"git.cs.nctu.edu.tw/calee/sctp"
	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/amf/msgtypes/ngapmsgtypes"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	"github.com/omec-project/ngap"
	"github.com/omec-project/ngap/ngapType"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

var tracer = otel.Tracer("amf/ngap")

func DispatchLb(ctx ctxt.Context, sctplbMsg *sdcoreAmfServer.SctplbMessage, Amf2RanMsgChan chan *sdcoreAmfServer.AmfMessage) {
	logger.NgapLog.Infof("dispatchLb GnbId:%v GnbIp: %v %T", sctplbMsg.GnbId, sctplbMsg.GnbIpAddr, Amf2RanMsgChan)
	var ran *context.AmfRan
	amfSelf := context.AMF_Self()

	if sctplbMsg.GnbId != "" {
		var ok bool
		ran, ok = amfSelf.AmfRanFindByGnbId(sctplbMsg.GnbId)
		if !ok {
			logger.NgapLog.Infof("create a new NG connection for: %s", sctplbMsg.GnbId)
			ran = amfSelf.NewAmfRanId(sctplbMsg.GnbId)
			ran.Amf2RanMsgChan = Amf2RanMsgChan
			logger.NgapLog.Infof("dispatchLb, Create new Amf RAN", sctplbMsg.GnbId)
		}
	} else if sctplbMsg.GnbIpAddr != "" {
		logger.NgapLog.Infoln("GnbIpAddress received but no GnbId")
		ran = &context.AmfRan{}
		ran.SupportedTAList = context.NewSupportedTAIList()
		ran.Amf2RanMsgChan = Amf2RanMsgChan
		ran.Log = logger.NgapLog.With(logger.FieldRanAddr, sctplbMsg.GnbIpAddr)
		ran.GnbIp = sctplbMsg.GnbIpAddr
		logger.NgapLog.Infoln("dispatchLb, Create new Amf RAN with GnbIpAddress", sctplbMsg.GnbIpAddr)
	}

	if len(sctplbMsg.Msg) == 0 {
		logger.NgapLog.Infof("dispatchLb, Message of size 0 - ", sctplbMsg.GnbId)
		ran.Log.Infoln("RAN close the connection")
		ran.Remove()
		return
	}

	pdu, err := ngap.Decoder(sctplbMsg.Msg)
	if err != nil {
		ran.Log.Errorf("NGAP decode error: %+v", err)
		logger.NgapLog.Infoln("dispatchLb, decode Messgae error", sctplbMsg.GnbId)
		return
	}

	ranUe, ngapId := FetchRanUeContext(ran, pdu)
	if ngapId != nil {
		//ranUe.Log.Debugln("RanUe RanNgapId AmfNgapId: ", ranUe.RanUeNgapId, ranUe.AmfUeNgapId)
		/* checking whether same AMF instance can handle this message */
		/* redirect it to correct owner if required */
		if amfSelf.EnableDbStore {
			id, err := amfSelf.Drsm.FindOwnerInt32ID(int32(ngapId.Value))
			if id == nil || err != nil {
				ran.Log.Warnf("dispatchLb, Couldn't find owner for amfUeNgapid: %v", ngapId.Value)
			} else if id.PodName != os.Getenv("HOSTNAME") {
				rsp := &sdcoreAmfServer.AmfMessage{}
				rsp.VerboseMsg = "Redirect Msg From AMF Pod !"
				rsp.Msgtype = sdcoreAmfServer.MsgType_REDIRECT_MSG
				rsp.AmfId = os.Getenv("HOSTNAME")
				/* TODO set only pod name, for this release setting pod ip to simplify logic in sctplb */
				logger.NgapLog.Infof("dispatchLb, amfNgapId: %v is not for this amf instance, redirect to amf instance: %v %v", ngapId.Value, id.PodName, id.PodIp)
				rsp.RedirectId = id.PodIp
				rsp.GnbId = ran.GnbId
				rsp.Msg = make([]byte, len(sctplbMsg.Msg))
				copy(rsp.Msg, sctplbMsg.Msg)
				ran.Amf2RanMsgChan = Amf2RanMsgChan
				ran.Amf2RanMsgChan <- rsp
				if ranUe != nil && ranUe.AmfUe != nil {
					ranUe.AmfUe.Remove()
				}
				if ranUe != nil {
					if err := ranUe.Remove(); err != nil {
						ran.Log.Errorf("could not remove ranUe: %v", err)
					}
				}
				return
			} else {
				ran.Log.Debugf("DispatchLb, amfNgapId: %v for this amf instance", ngapId.Value)
			}
		}
	}

	/* uecontext is found, submit the message to transaction queue*/
	if ranUe != nil && ranUe.AmfUe != nil {
		ranUe.AmfUe.SetEventChannel(ctx, NgapMsgHandler)
		// ranUe.AmfUe.TxLog.Infoln("Uecontext found. queuing ngap message to uechannel")
		ranUe.AmfUe.EventChannel.UpdateNgapHandler(NgapMsgHandler)
		ngapMsg := context.NgapMsg{
			Ran:       ran,
			NgapMsg:   pdu,
			SctplbMsg: sctplbMsg,
		}

		ranUe.AmfUe.EventChannel.SubmitMessage(ngapMsg)
	} else {
		go DispatchNgapMsg(ctx, ran, pdu, sctplbMsg)
	}
}

func Dispatch(conn net.Conn, msg []byte) {
	var ran *context.AmfRan
	amfSelf := context.AMF_Self()

	ctx := ctxt.Background()

	ran, ok := amfSelf.AmfRanFindByConn(conn)
	if !ok {
		logger.NgapLog.Infof("Create a new NG connection for: %s", conn.RemoteAddr().String())
		ran = amfSelf.NewAmfRan(conn)
	}

	if len(msg) == 0 {
		ran.Log.Infoln("RAN close the connection")
		ran.Remove()
		return
	}

	pdu, err := ngap.Decoder(msg)
	if err != nil {
		ran.Log.Errorf("NGAP decode error: %+v", err)
		return
	}

	ranUe, _ := FetchRanUeContext(ran, pdu)

	/* uecontext is found, submit the message to transaction queue*/
	if ranUe != nil && ranUe.AmfUe != nil {
		ranUe.AmfUe.SetEventChannel(ctx, NgapMsgHandler)
		ranUe.AmfUe.TxLog.Infoln("Uecontext found. queuing ngap message to uechannel")
		ranUe.AmfUe.EventChannel.UpdateNgapHandler(NgapMsgHandler)
		ngapMsg := context.NgapMsg{
			Ran:       ran,
			NgapMsg:   pdu,
			SctplbMsg: nil,
		}
		if ranUe.Ran.GnbId == ran.GnbId {
			ranUe.AmfUe.TxLog.Infoln("gnbid match")
			ranUe.Ran.Conn = conn
		} else {
			ranUe.AmfUe.TxLog.Infoln("gnbid differ")
			ranUe.AmfUe.TxLog.Infof("In case of Xn handover source RAN gNB id:%s, target RAN gNB id:%s", ranUe.Ran.GnbId, ran.GnbId)
		}
		ranUe.AmfUe.EventChannel.SubmitMessage(ngapMsg)
	} else {
		go DispatchNgapMsg(ctx, ran, pdu, nil)
	}
}

func NgapMsgHandler(ue *context.AmfUe, msg context.NgapMsg) {
	DispatchNgapMsg(ctxt.Background(), msg.Ran, msg.NgapMsg, msg.SctplbMsg)
}

func DispatchNgapMsg(ctx ctxt.Context, ran *context.AmfRan, pdu *ngapType.NGAPPDU, sctplbMsg *sdcoreAmfServer.SctplbMessage) {
	var code int64
	switch pdu.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		if pdu.InitiatingMessage != nil {
			code = pdu.InitiatingMessage.ProcedureCode.Value
		}
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		if pdu.SuccessfulOutcome != nil {
			code = pdu.SuccessfulOutcome.ProcedureCode.Value
		}
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		if pdu.UnsuccessfulOutcome != nil {
			code = pdu.UnsuccessfulOutcome.ProcedureCode.Value
		}
	}
	procName := ngapType.ProcedureName(code)

	peer := "unknown"
	if ran != nil && ran.Conn != nil {
		if addr := ran.Conn.RemoteAddr(); addr != nil {
			peer = addr.String()
		}
	}

	if procName == "" {
		procName = fmt.Sprintf("UnknownProcedureCode_%d", code)
		logger.AppLog.Warnf("Encountered unknown NGAP procedure code: %d from RAN: %s", code, peer)
	}

	spanName := fmt.Sprintf("AMF NGAP %s", procName)
	ctx, span := tracer.Start(ctx, spanName,
		trace.WithAttributes(
			attribute.String("net.peer", peer),
			attribute.String("ngap.pdu_present", fmt.Sprintf("%d", pdu.Present)),
			attribute.String("ngap.procedureCode", procName),
		),
		trace.WithSpanKind(trace.SpanKindServer),
	)
	defer span.End()

	switch pdu.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		initiatingMessage := pdu.InitiatingMessage
		if initiatingMessage == nil {
			ran.Log.Errorln("Initiating Message is nil")
			return
		}

		metrics.IncrementNgapMsgStats(context.AMF_Self().NfId,
			ngapmsgtypes.NgapMsg[initiatingMessage.ProcedureCode.Value],
			"in",
			"",
			"")
		switch initiatingMessage.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGSetup:
			HandleNGSetupRequest(ran, pdu)
		case ngapType.ProcedureCodeInitialUEMessage:
			HandleInitialUEMessage(ctx, ran, pdu, sctplbMsg)
		case ngapType.ProcedureCodeUplinkNASTransport:
			HandleUplinkNasTransport(ctx, ran, pdu)
		case ngapType.ProcedureCodeNGReset:
			HandleNGReset(ran, pdu)
		case ngapType.ProcedureCodeHandoverCancel:
			HandleHandoverCancel(ctx, ran, pdu)
		case ngapType.ProcedureCodeUEContextReleaseRequest:
			HandleUEContextReleaseRequest(ctx, ran, pdu)
		case ngapType.ProcedureCodeNASNonDeliveryIndication:
			HandleNasNonDeliveryIndication(ctx, ran, pdu)
		case ngapType.ProcedureCodeLocationReportingFailureIndication:
			HandleLocationReportingFailureIndication(ran, pdu)
		case ngapType.ProcedureCodeErrorIndication:
			HandleErrorIndication(ran, pdu)
		case ngapType.ProcedureCodeUERadioCapabilityInfoIndication:
			HandleUERadioCapabilityInfoIndication(ran, pdu)
		case ngapType.ProcedureCodeHandoverNotification:
			HandleHandoverNotify(ctx, ran, pdu)
		case ngapType.ProcedureCodeHandoverPreparation:
			HandleHandoverRequired(ctx, ran, pdu)
		case ngapType.ProcedureCodeRANConfigurationUpdate:
			HandleRanConfigurationUpdate(ran, pdu)
		case ngapType.ProcedureCodeRRCInactiveTransitionReport:
			HandleRRCInactiveTransitionReport(ran, pdu)
		case ngapType.ProcedureCodePDUSessionResourceNotify:
			HandlePDUSessionResourceNotify(ctx, ran, pdu)
		case ngapType.ProcedureCodePathSwitchRequest:
			HandlePathSwitchRequest(ctx, ran, pdu)
		case ngapType.ProcedureCodeLocationReport:
			HandleLocationReport(ran, pdu)
		case ngapType.ProcedureCodeUplinkUEAssociatedNRPPaTransport:
			HandleUplinkUEAssociatedNRPPATransport(ran, pdu)
		case ngapType.ProcedureCodeUplinkRANConfigurationTransfer:
			HandleUplinkRanConfigurationTransfer(ran, pdu)
		case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
			HandlePDUSessionResourceModifyIndication(ctx, ran, pdu)
		case ngapType.ProcedureCodeCellTrafficTrace:
			HandleCellTrafficTrace(ran, pdu)
		case ngapType.ProcedureCodeUplinkRANStatusTransfer:
			HandleUplinkRanStatusTransfer(ran, pdu)
		case ngapType.ProcedureCodeUplinkNonUEAssociatedNRPPaTransport:
			HandleUplinkNonUEAssociatedNRPPATransport(ran, pdu)
		default:
			ran.Log.Warnf("Not implemented(choice: %d, procedureCode: %d)", pdu.Present, initiatingMessage.ProcedureCode.Value)
		}
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		successfulOutcome := pdu.SuccessfulOutcome
		if successfulOutcome == nil {
			ran.Log.Errorln("successful Outcome is nil")
			return
		}
		metrics.IncrementNgapMsgStats(context.AMF_Self().NfId,
			ngapmsgtypes.NgapMsg[successfulOutcome.ProcedureCode.Value],
			"in",
			"",
			"")
		switch successfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeNGReset:
			HandleNGResetAcknowledge(ran, pdu)
		case ngapType.ProcedureCodeUEContextRelease:
			HandleUEContextReleaseComplete(ctx, ran, pdu)
		case ngapType.ProcedureCodePDUSessionResourceRelease:
			HandlePDUSessionResourceReleaseResponse(ctx, ran, pdu)
		case ngapType.ProcedureCodeUERadioCapabilityCheck:
			HandleUERadioCapabilityCheckResponse(ran, pdu)
		case ngapType.ProcedureCodeAMFConfigurationUpdate:
			HandleAMFconfigurationUpdateAcknowledge(ran, pdu)
		case ngapType.ProcedureCodeInitialContextSetup:
			HandleInitialContextSetupResponse(ctx, ran, pdu)
		case ngapType.ProcedureCodeUEContextModification:
			HandleUEContextModificationResponse(ran, pdu)
		case ngapType.ProcedureCodePDUSessionResourceSetup:
			HandlePDUSessionResourceSetupResponse(ctx, ran, pdu)
		case ngapType.ProcedureCodePDUSessionResourceModify:
			HandlePDUSessionResourceModifyResponse(ctx, ran, pdu)
		case ngapType.ProcedureCodeHandoverResourceAllocation:
			HandleHandoverRequestAcknowledge(ctx, ran, pdu)
		default:
			ran.Log.Warnf("Not implemented(choice: %d, procedureCode: %d)", pdu.Present, successfulOutcome.ProcedureCode.Value)
		}
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		unsuccessfulOutcome := pdu.UnsuccessfulOutcome
		if unsuccessfulOutcome == nil {
			ran.Log.Errorln("unsuccessful Outcome is nil")
			return
		}
		metrics.IncrementNgapMsgStats(context.AMF_Self().NfId,
			ngapmsgtypes.NgapMsg[unsuccessfulOutcome.ProcedureCode.Value],
			"in",
			"",
			"")
		switch unsuccessfulOutcome.ProcedureCode.Value {
		case ngapType.ProcedureCodeAMFConfigurationUpdate:
			HandleAMFconfigurationUpdateFailure(ran, pdu)
		case ngapType.ProcedureCodeInitialContextSetup:
			HandleInitialContextSetupFailure(ctx, ran, pdu)
		case ngapType.ProcedureCodeUEContextModification:
			HandleUEContextModificationFailure(ran, pdu)
		case ngapType.ProcedureCodeHandoverResourceAllocation:
			HandleHandoverFailure(ctx, ran, pdu)
		default:
			ran.Log.Warnf("Not implemented(choice: %d, procedureCode: %d)", pdu.Present, unsuccessfulOutcome.ProcedureCode.Value)
		}
	}
}

func HandleSCTPNotification(conn net.Conn, notification sctp.Notification) {
	amfSelf := context.AMF_Self()

	logger.NgapLog.Infof("Handle SCTP Notification[addr: %+v]", conn.RemoteAddr())

	ran, ok := amfSelf.AmfRanFindByConn(conn)
	if !ok {
		logger.NgapLog.Warnf("RAN context has been removed[addr: %+v]", conn.RemoteAddr())
		return
	}

	// Removing Stale Connections in AmfRanPool
	amfSelf.AmfRanPool.Range(func(key, value interface{}) bool {
		amfRan := value.(*context.AmfRan)

		conn := amfRan.Conn.(*sctp.SCTPConn)
		errorConn := sctp.NewSCTPConn(-1, nil)
		if reflect.DeepEqual(conn, errorConn) {
			amfRan.Remove()
			ran.Log.Infoln("removed stale entry in AmfRan pool")
		}
		return true
	})

	switch notification.Type() {
	case sctp.SCTP_ASSOC_CHANGE:
		ran.Log.Infoln("SCTP_ASSOC_CHANGE notification")
		event := notification.(*sctp.SCTPAssocChangeEvent)
		switch event.State() {
		case sctp.SCTP_COMM_LOST:
			ran.Log.Infoln("SCTP state is SCTP_COMM_LOST, close the connection")
			ran.Remove()
		case sctp.SCTP_SHUTDOWN_COMP:
			ran.Log.Infoln("SCTP state is SCTP_SHUTDOWN_COMP, close the connection")
			ran.Remove()
		default:
			ran.Log.Warnf("SCTP state[%+v] is not handled", event.State())
		}
	case sctp.SCTP_SHUTDOWN_EVENT:
		ran.Log.Infoln("SCTP_SHUTDOWN_EVENT notification, close the connection")
		ran.Remove()
	default:
		ran.Log.Warnf("Non handled notification type: 0x%x", notification.Type())
	}
}

func HandleSCTPNotificationLb(gnbId string) {
	logger.NgapLog.Infof("Handle SCTP Notification[GnbId: %+v]", gnbId)

	amfSelf := context.AMF_Self()
	ran, ok := amfSelf.AmfRanFindByGnbId(gnbId)
	if !ok {
		logger.NgapLog.Warnf("RAN context has been removed[gnbId: %+v]", gnbId)
		return
	}

	// Removing Stale Connections in AmfRanPool
	amfSelf.AmfRanPool.Range(func(key, value interface{}) bool {
		amfRan := value.(*context.AmfRan)

		if amfRan.GnbId == gnbId {
			amfRan.Remove()
			ran.Log.Infoln("removed stale entry in AmfRan pool")
		}
		return true
	})

	ran.Log.Infoln("SCTP state is SCTP_SHUTDOWN_COMP, close the connection")
	ran.Remove()
}
