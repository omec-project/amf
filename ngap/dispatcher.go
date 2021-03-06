// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package ngap

import (
	"net"
	"reflect"

	"git.cs.nctu.edu.tw/calee/sctp"

	"fmt"
	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/amf/msgtypes/ngapmsgtypes"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	"github.com/omec-project/ngap"
	"github.com/omec-project/ngap/ngapType"
)

func DispatchLb(remoteAddr string, msg []byte, Amf2RanMsgChan chan *sdcoreAmfServer.Message) {
	fmt.Printf("DispatchLb %v %T", remoteAddr, Amf2RanMsgChan)
	var ran *context.AmfRan
	amfSelf := context.AMF_Self()

	ran, ok := amfSelf.AmfRanFindByAddr(remoteAddr)
	if !ok {
		logger.NgapLog.Infof("Create a new NG connection for: %s", remoteAddr)
		ran = amfSelf.NewAmfRanAddr(remoteAddr)
		ran.Amf2RanMsgChan = Amf2RanMsgChan
		fmt.Println("DispatchLb, Create new Amf RAN ", remoteAddr)
	}

	if len(msg) == 0 {
		fmt.Println("DispatchLb, Messgae of size 0 -  ", remoteAddr)
		ran.Log.Infof("RAN close the connection.")
		ran.Remove()
		return
	}

	pdu, err := ngap.Decoder(msg)
	if err != nil {
		ran.Log.Errorf("NGAP decode error : %+v", err)
		fmt.Println("DispatchLb, decode Messgae error ", remoteAddr)
		return
	}

	ranUe := FetchRanUeContext(ran, pdu)

	/* uecontext is found, submit the message to transaction queue*/
	if ranUe != nil && ranUe.AmfUe != nil {
		if ranUe.AmfUe.EventChannel == nil {
			ran.Log.Errorf("AmfUe EventChannel is not exist")
			return
		}
		ranUe.AmfUe.TxLog.Infof("Uecontext found. queuing ngap message to uechannel")
		ranUe.AmfUe.EventChannel.UpdateNgapHandler(NgapMsgHandler)
		ngapMsg := context.NgapMsg{
			Ran:     ran,
			NgapMsg: pdu,
		}
		ranUe.AmfUe.EventChannel.SubmitMessage(ngapMsg)
	} else {
		go DispatchNgapMsg(ran, pdu)
	}
}

func Dispatch(conn net.Conn, msg []byte) {
	var ran *context.AmfRan
	amfSelf := context.AMF_Self()

	ran, ok := amfSelf.AmfRanFindByConn(conn)
	if !ok {
		logger.NgapLog.Infof("Create a new NG connection for: %s", conn.RemoteAddr().String())
		ran = amfSelf.NewAmfRan(conn)
	}

	if len(msg) == 0 {
		ran.Log.Infof("RAN close the connection.")
		ran.Remove()
		return
	}

	pdu, err := ngap.Decoder(msg)
	if err != nil {
		ran.Log.Errorf("NGAP decode error : %+v", err)
		return
	}

	ranUe := FetchRanUeContext(ran, pdu)

	/* uecontext is found, submit the message to transaction queue*/
	if ranUe != nil && ranUe.AmfUe != nil {
		if ranUe.AmfUe.EventChannel == nil {
			ran.Log.Errorf("AmfUe EventChannel is not exist")
			return
		}
		ranUe.AmfUe.TxLog.Infof("Uecontext found. queuing ngap message to uechannel")
		ranUe.AmfUe.EventChannel.UpdateNgapHandler(NgapMsgHandler)
		ngapMsg := context.NgapMsg{
			Ran:     ran,
			NgapMsg: pdu,
		}
		ranUe.AmfUe.EventChannel.SubmitMessage(ngapMsg)
	} else {
		go DispatchNgapMsg(ran, pdu)
	}
}

func NgapMsgHandler(ue *context.AmfUe, msg context.NgapMsg) {
	DispatchNgapMsg(msg.Ran, msg.NgapMsg)
}

func DispatchNgapMsg(ran *context.AmfRan, pdu *ngapType.NGAPPDU) {

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
			HandleInitialUEMessage(ran, pdu)
		case ngapType.ProcedureCodeUplinkNASTransport:
			HandleUplinkNasTransport(ran, pdu)
		case ngapType.ProcedureCodeNGReset:
			HandleNGReset(ran, pdu)
		case ngapType.ProcedureCodeHandoverCancel:
			HandleHandoverCancel(ran, pdu)
		case ngapType.ProcedureCodeUEContextReleaseRequest:
			HandleUEContextReleaseRequest(ran, pdu)
		case ngapType.ProcedureCodeNASNonDeliveryIndication:
			HandleNasNonDeliveryIndication(ran, pdu)
		case ngapType.ProcedureCodeLocationReportingFailureIndication:
			HandleLocationReportingFailureIndication(ran, pdu)
		case ngapType.ProcedureCodeErrorIndication:
			HandleErrorIndication(ran, pdu)
		case ngapType.ProcedureCodeUERadioCapabilityInfoIndication:
			HandleUERadioCapabilityInfoIndication(ran, pdu)
		case ngapType.ProcedureCodeHandoverNotification:
			HandleHandoverNotify(ran, pdu)
		case ngapType.ProcedureCodeHandoverPreparation:
			HandleHandoverRequired(ran, pdu)
		case ngapType.ProcedureCodeRANConfigurationUpdate:
			HandleRanConfigurationUpdate(ran, pdu)
		case ngapType.ProcedureCodeRRCInactiveTransitionReport:
			HandleRRCInactiveTransitionReport(ran, pdu)
		case ngapType.ProcedureCodePDUSessionResourceNotify:
			HandlePDUSessionResourceNotify(ran, pdu)
		case ngapType.ProcedureCodePathSwitchRequest:
			HandlePathSwitchRequest(ran, pdu)
		case ngapType.ProcedureCodeLocationReport:
			HandleLocationReport(ran, pdu)
		case ngapType.ProcedureCodeUplinkUEAssociatedNRPPaTransport:
			HandleUplinkUEAssociatedNRPPATransport(ran, pdu)
		case ngapType.ProcedureCodeUplinkRANConfigurationTransfer:
			HandleUplinkRanConfigurationTransfer(ran, pdu)
		case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
			HandlePDUSessionResourceModifyIndication(ran, pdu)
		case ngapType.ProcedureCodeCellTrafficTrace:
			HandleCellTrafficTrace(ran, pdu)
		case ngapType.ProcedureCodeUplinkRANStatusTransfer:
			HandleUplinkRanStatusTransfer(ran, pdu)
		case ngapType.ProcedureCodeUplinkNonUEAssociatedNRPPaTransport:
			HandleUplinkNonUEAssociatedNRPPATransport(ran, pdu)
		default:
			ran.Log.Warnf("Not implemented(choice:%d, procedureCode:%d)\n", pdu.Present, initiatingMessage.ProcedureCode.Value)
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
			HandleUEContextReleaseComplete(ran, pdu)
		case ngapType.ProcedureCodePDUSessionResourceRelease:
			HandlePDUSessionResourceReleaseResponse(ran, pdu)
		case ngapType.ProcedureCodeUERadioCapabilityCheck:
			HandleUERadioCapabilityCheckResponse(ran, pdu)
		case ngapType.ProcedureCodeAMFConfigurationUpdate:
			HandleAMFconfigurationUpdateAcknowledge(ran, pdu)
		case ngapType.ProcedureCodeInitialContextSetup:
			HandleInitialContextSetupResponse(ran, pdu)
		case ngapType.ProcedureCodeUEContextModification:
			HandleUEContextModificationResponse(ran, pdu)
		case ngapType.ProcedureCodePDUSessionResourceSetup:
			HandlePDUSessionResourceSetupResponse(ran, pdu)
		case ngapType.ProcedureCodePDUSessionResourceModify:
			HandlePDUSessionResourceModifyResponse(ran, pdu)
		case ngapType.ProcedureCodeHandoverResourceAllocation:
			HandleHandoverRequestAcknowledge(ran, pdu)
		default:
			ran.Log.Warnf("Not implemented(choice:%d, procedureCode:%d)\n", pdu.Present, successfulOutcome.ProcedureCode.Value)
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
			HandleInitialContextSetupFailure(ran, pdu)
		case ngapType.ProcedureCodeUEContextModification:
			HandleUEContextModificationFailure(ran, pdu)
		case ngapType.ProcedureCodeHandoverResourceAllocation:
			HandleHandoverFailure(ran, pdu)
		default:
			ran.Log.Warnf("Not implemented(choice:%d, procedureCode:%d)\n", pdu.Present, unsuccessfulOutcome.ProcedureCode.Value)
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

	//Removing Stale Connections in AmfRanPool
	amfSelf.AmfRanPool.Range(func(key, value interface{}) bool {
		amfRan := value.(*context.AmfRan)

		conn := amfRan.Conn.(*sctp.SCTPConn)
		errorConn := sctp.NewSCTPConn(-1, nil)
		if reflect.DeepEqual(conn, errorConn) == true {
			amfRan.Remove()
			ran.Log.Infof("removed stale entry in AmfRan pool")
		}
		return true
	})

	switch notification.Type() {
	case sctp.SCTP_ASSOC_CHANGE:
		ran.Log.Infof("SCTP_ASSOC_CHANGE notification")
		event := notification.(*sctp.SCTPAssocChangeEvent)
		switch event.State() {
		case sctp.SCTP_COMM_LOST:
			ran.Log.Infof("SCTP state is SCTP_COMM_LOST, close the connection")
			ran.Remove()
		case sctp.SCTP_SHUTDOWN_COMP:
			ran.Log.Infof("SCTP state is SCTP_SHUTDOWN_COMP, close the connection")
			ran.Remove()
		default:
			ran.Log.Warnf("SCTP state[%+v] is not handled", event.State())
		}
	case sctp.SCTP_SHUTDOWN_EVENT:
		ran.Log.Infof("SCTP_SHUTDOWN_EVENT notification, close the connection")
		ran.Remove()
	default:
		ran.Log.Warnf("Non handled notification type: 0x%x", notification.Type())
	}
}

func HandleSCTPNotificationLb(remoteAddr string) {

	logger.NgapLog.Infof("Handle SCTP Notification[addr: %+v]", remoteAddr)

	amfSelf := context.AMF_Self()
	ran, ok := amfSelf.AmfRanFindByAddr(remoteAddr)
	if !ok {
		logger.NgapLog.Warnf("RAN context has been removed[addr: %+v]", remoteAddr)
		return
	}

	//Removing Stale Connections in AmfRanPool
	amfSelf.AmfRanPool.Range(func(key, value interface{}) bool {
		amfRan := value.(*context.AmfRan)

		if amfRan.GnbIp == remoteAddr {
			amfRan.Remove()
			ran.Log.Infof("removed stale entry in AmfRan pool")
		}
		return true
	})

	ran.Log.Infof("SCTP state is SCTP_SHUTDOWN_COMP, close the connection")
	ran.Remove()
}
