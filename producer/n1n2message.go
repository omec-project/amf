// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package producer

import (
	ctxt "context"
	"io"
	"net/http"
	"strconv"

	"github.com/omec-project/amf/context"
	gmm_message "github.com/omec-project/amf/gmm/message"
	"github.com/omec-project/amf/logger"
	ngap_message "github.com/omec-project/amf/ngap/message"
	"github.com/omec-project/amf/producer/callback"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/ngap/v2/aper"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
)

func ProducerHandler(ctx ctxt.Context, s1, s2 string, msg interface{}) (interface{}, string, interface{}, interface{}) {
	if msg == nil {
		r1, r2 := N1N2MessageTransferStatusProcedure(s1, s2)
		return r1, "", r2, nil
	}
	switch msg := msg.(type) {
	case models.N1N2MessageTransferRequest:
		return N1N2MessageTransferProcedure(s1, s2, msg)
	case models.UeN1N2InfoSubscriptionCreateData:
		r1, r2 := N1N2MessageSubscribeProcedure(s1, msg)
		return r1, "", r2, nil
	}

	return nil, "", nil, nil
}

// TS23502 4.2.3.3, 4.2.4.3, 4.3.2.2, 4.3.2.3, 4.3.3.2, 4.3.7
func HandleN1N2MessageTransferRequest(request *httpwrapper.Request) *httpwrapper.Response {
	var ue *context.AmfUe
	var ok bool
	var problemDetails *models.ProblemDetails
	logger.ProducerLog.Infoln("handle N1N2 Message Transfer Request")

	n1n2MessageTransferRequest := request.Body.(models.N1N2MessageTransferRequest)
	ueContextID := request.Params["ueContextId"]
	reqUri := request.Params["reqUri"]

	amfSelf := context.AMF_Self()

	if ue, ok = amfSelf.AmfUeFindByUeContextID(ueContextID); !ok {
		problemDetails = utils.ProblemDetailsContextNotFound("UE context not found")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}

	// If EventChannel is nil (e.g. UE context restored from DB after restart),
	// call the procedure directly — it already handles CM-IDLE/paging correctly.
	if ue.EventChannel == nil {
		logger.ProducerLog.Warnln("EventChannel is nil for UE, invoking N1N2MessageTransferProcedure directly")
		rspData, locHeader, pd, txErr := N1N2MessageTransferProcedure(ueContextID, reqUri, n1n2MessageTransferRequest)
		if pd != nil {
			return httpwrapper.NewResponse(int(pd.GetStatus()), nil, pd)
		} else if txErr != nil {
			return httpwrapper.NewResponse(int(txErr.Error.GetStatus()), nil, txErr)
		} else if rspData != nil {
			switch rspData.Cause {
			case models.N1N2MESSAGETRANSFERCAUSE_N1_MSG_NOT_TRANSFERRED:
				fallthrough
			case models.N1N2MESSAGETRANSFERCAUSE_N1_N2_TRANSFER_INITIATED:
				return httpwrapper.NewResponse(http.StatusOK, nil, rspData)
			case models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE:
				headers := http.Header{"Location": {locHeader}}
				return httpwrapper.NewResponse(http.StatusAccepted, headers, rspData)
			}
		}
		problemDetails = utils.ProblemDetailsUnspecified()
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}

	sbiMsg := context.SbiMsg{
		UeContextId: ueContextID,
		ReqUri:      reqUri,
		Msg:         n1n2MessageTransferRequest,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	var n1n2MessageTransferRspData *models.N1N2MessageTransferRspData
	var transferErr *models.N1N2MessageTransferError
	ue.EventChannel.UpdateSbiHandler(ProducerHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result
	if msg.RespData != nil {
		n1n2MessageTransferRspData = msg.RespData.(*models.N1N2MessageTransferRspData)
	}
	locationHeader := msg.LocationHeader
	if msg.ProblemDetails != nil {
		problemDetails = msg.ProblemDetails.(*models.ProblemDetails)
	}
	if msg.TransferErr != nil {
		transferErr = msg.TransferErr.(*models.N1N2MessageTransferError)
	}
	//		n1n2MessageTransferRspData, locationHeader, problemDetails, transferErr := N1N2MessageTransferProcedure(
	//			ueContextID, reqUri, n1n2MessageTransferRequest)

	if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	} else if transferErr != nil {
		return httpwrapper.NewResponse(int(transferErr.Error.GetStatus()), nil, transferErr)
	} else if n1n2MessageTransferRspData != nil {
		switch n1n2MessageTransferRspData.Cause {
		case models.N1N2MESSAGETRANSFERCAUSE_N1_MSG_NOT_TRANSFERRED:
			fallthrough
		case models.N1N2MESSAGETRANSFERCAUSE_N1_N2_TRANSFER_INITIATED:
			return httpwrapper.NewResponse(http.StatusOK, nil, n1n2MessageTransferRspData)
		case models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE:
			headers := http.Header{
				"Location": {locationHeader},
			}
			return httpwrapper.NewResponse(http.StatusAccepted, headers, n1n2MessageTransferRspData)
		}
	}

	problemDetails = utils.ProblemDetailsUnspecified()
	return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
}

// There are 4 possible return value for this function:
//   - n1n2MessageTransferRspData: if AMF handle N1N2MessageTransfer Request successfully.
//   - locationHeader: if response status code is 202, then it will return a non-empty string location header for
//     response
//   - problemDetails: if AMF reject the request due to application error, e.g. UE context not found.
//   - TransferErr: if AMF reject the request due to procedure error, e.g. UE has an ongoing procedure.
//
// see TS 29.518 6.1.3.5.3.1 for more details.
func N1N2MessageTransferProcedure(ueContextID string, reqUri string,
	n1n2MessageTransferRequest models.N1N2MessageTransferRequest) (
	n1n2MessageTransferRspData *models.N1N2MessageTransferRspData,
	locationHeader string, problemDetails *models.ProblemDetails,
	transferErr *models.N1N2MessageTransferError,
) {
	var (
		requestData = n1n2MessageTransferRequest.JsonData

		ue        *context.AmfUe
		ok        bool
		smContext *context.SmContext
		n1MsgType uint8
		anType    = models.ACCESSTYPE__3_GPP_ACCESS
	)

	amfSelf := context.AMF_Self()

	if ue, ok = amfSelf.AmfUeFindByUeContextID(ueContextID); !ok {
		problemDetails = utils.ProblemDetailsContextNotFound("UE context not found")
		return nil, "", problemDetails, nil
	}

	var n2Info []byte
	if requestData.N2InfoContainer != nil {
		if binaryDataN2Information := n1n2MessageTransferRequest.GetBinaryDataN2Information(); binaryDataN2Information != nil {
			binaryN2Info, err := io.ReadAll(binaryDataN2Information)
			if err != nil {
				ue.ProducerLog.Errorf("read binaryDataN2Information failed: %+v", err)
				n2Info = nil
			} else {
				n2Info = binaryN2Info
			}
		}
	}

	var n1Msg []byte
	if requestData.N1MessageContainer != nil {
		if binaryDataN1Message := n1n2MessageTransferRequest.GetBinaryDataN1Message(); binaryDataN1Message != nil {
			binaryN1Msg, err := io.ReadAll(binaryDataN1Message)
			if err != nil {
				ue.ProducerLog.Errorf("read binaryDataN1Message failed: %+v", err)
				n1Msg = nil
			} else {
				n1Msg = binaryN1Msg
			}
		}
	}

	ue.ProducerLog.Debugf(
		"decoded N1N2 transfer request: hasN1Container=%t hasN2Container=%t n1Len=%d n2Len=%d pduSessionId=%d",
		requestData.N1MessageContainer != nil,
		requestData.N2InfoContainer != nil,
		len(n1Msg),
		len(n2Info),
		requestData.GetPduSessionId(),
	)

	if requestData.N1MessageContainer != nil {
		switch requestData.N1MessageContainer.GetN1MessageClass() {
		case models.N1MESSAGECLASS_SM:
			ue.ProducerLog.Debugf("receive N1 SM Message (PDU Session ID: %d)", requestData.GetPduSessionId())
			n1MsgType = nasMessage.PayloadContainerTypeN1SMInfo
			if smContext, ok = ue.SmContextFindByPDUSessionID(requestData.GetPduSessionId()); !ok {
				problemDetails = utils.ProblemDetailsContextNotFound("SM context not found")
				return nil, "", problemDetails, nil
			} else {
				anType = smContext.AccessType()
			}
		case models.N1MESSAGECLASS_SMS:
			n1MsgType = nasMessage.PayloadContainerTypeSMS
		case models.N1MESSAGECLASS_LPP:
			n1MsgType = nasMessage.PayloadContainerTypeLPP
		case models.N1MESSAGECLASS_UPDP:
			n1MsgType = nasMessage.PayloadContainerTypeUEPolicy
		default:
		}
	}

	if requestData.N2InfoContainer != nil {
		switch requestData.N2InfoContainer.GetN2InformationClass() {
		case models.N2INFORMATIONCLASS_SM:
			ue.ProducerLog.Debugf("receive N2 SM Message (PDU Session ID: %d)", requestData.GetPduSessionId())
			if smContext == nil {
				if smContext, ok = ue.SmContextFindByPDUSessionID(requestData.GetPduSessionId()); !ok {
					problemDetails = utils.ProblemDetailsContextNotFound("SM context not found")
					return nil, "", problemDetails, nil
				} else {
					anType = smContext.AccessType()
				}
			}
		default:
			ue.ProducerLog.Warnf("N2 Information type [%s] is not supported", requestData.N2InfoContainer.GetN2InformationClass())
			problemDetails = utils.ProblemDetailsWithCause("Not implemented", http.StatusNotImplemented, "N2 Information type not supported", utils.CauseNotImplemented)
			return nil, "", problemDetails, nil
		}
	}

	onGoing := ue.GetOnGoing(anType)
	// 4xx response cases
	// TODO: Error Status 307, 403 in TS29.518 Table 6.1.3.5.3.1-3
	switch onGoing.Procedure {
	case context.OnGoingProcedurePaging:
		if requestData.GetPpi() == 0 || (onGoing.Ppi != 0 && onGoing.Ppi <= requestData.GetPpi()) {
			probDetails := utils.ProblemDetailsWithCause("Higher priority request ongoing", http.StatusConflict, "Higher priority request is ongoing", utils.CauseHigherPriorityRequestOngoing)
			transferErr = models.NewN1N2MessageTransferError(*probDetails)
			return nil, "", nil, transferErr
		}
		ue.T3513.Stop()
		callback.SendN1N2TransferFailureNotification(ue, models.N1N2MESSAGETRANSFERCAUSE_UE_NOT_RESPONDING)
	case context.OnGoingProcedureRegistration:
		probDetails := utils.ProblemDetailsWithCause("Registration ongoing", http.StatusConflict, "Registration is ongoing", utils.CauseTemporaryRejectRegistrationOngoing)
		transferErr = models.NewN1N2MessageTransferError(*probDetails)
		return nil, "", nil, transferErr
	case context.OnGoingProcedureN2Handover:
		probDetails := utils.ProblemDetailsWithCause("Handover ongoing", http.StatusConflict, "Handover is ongoing", utils.CauseTemporaryRejectHandoverOngoing)
		transferErr = models.NewN1N2MessageTransferError(*probDetails)
		return nil, "", nil, transferErr
	}

	// UE is CM-Connected
	if ue.CmConnect(anType) {
		var (
			nasPdu []byte
			err    error
		)
		if n1Msg != nil {
			nasPdu, err = gmm_message.BuildDLNASTransport(ue, anType, n1MsgType, n1Msg, uint8(requestData.GetPduSessionId()), nil, nil, 0)
			if err != nil {
				ue.ProducerLog.Errorf("build DL NAS Transport error: %+v", err)
				problemDetails = utils.ProblemDetailsSystemFailure(err.Error())
				return nil, "", problemDetails, nil
			}
			if n2Info == nil {
				ue.ProducerLog.Debugln("forward N1 Message to UE")
				ngap_message.SendDownlinkNasTransport(ue.RanUe[anType], nasPdu, nil)
				n1n2MessageTransferRspData = models.NewN1N2MessageTransferRspData(models.N1N2MESSAGETRANSFERCAUSE_N1_N2_TRANSFER_INITIATED)
				return n1n2MessageTransferRspData, "", nil, nil
			}
		}

		// TODO: only support transfer N2 SM information now
		if n2Info != nil {
			smInfo := requestData.N2InfoContainer.GetSmInfo()
			switch smInfo.N2InfoContent.GetNgapIeType() {
			case models.NGAPIETYPE_PDU_RES_SETUP_REQ:
				ue.ProducerLog.Debugln("AMF Transfer NGAP PDU Session Resource Setup Request from SMF")
				if ue.RanUe[anType].SentInitialContextSetupRequest {
					list := ngapType.PDUSessionResourceSetupListSUReq{}
					ngap_message.AppendPDUSessionResourceSetupListSUReq(&list, smInfo.PduSessionId, *smInfo.SNssai, nasPdu, n2Info)
					ngap_message.SendPDUSessionResourceSetupRequest(ue.RanUe[anType], nil, list)
				} else {
					list := ngapType.PDUSessionResourceSetupListCxtReq{}
					ngap_message.AppendPDUSessionResourceSetupListCxtReq(&list, smInfo.PduSessionId, *smInfo.SNssai, nasPdu, n2Info)
					ngap_message.SendInitialContextSetupRequest(ue, anType, nil, &list, nil, nil, nil)
					ue.RanUe[anType].SentInitialContextSetupRequest = true
				}
				n1n2MessageTransferRspData = models.NewN1N2MessageTransferRspData(models.N1N2MESSAGETRANSFERCAUSE_N1_N2_TRANSFER_INITIATED)
				// context.StoreContextInDB(ue)
				return n1n2MessageTransferRspData, "", nil, nil
			case models.NGAPIETYPE_PDU_RES_MOD_REQ:
				ue.ProducerLog.Debugln("AMF Transfer NGAP PDU Session Resource Modify Request from SMF")
				list := ngapType.PDUSessionResourceModifyListModReq{}
				ngap_message.AppendPDUSessionResourceModifyListModReq(&list, smInfo.PduSessionId, nasPdu, n2Info)
				ngap_message.SendPDUSessionResourceModifyRequest(ue.RanUe[anType], list)
				n1n2MessageTransferRspData = models.NewN1N2MessageTransferRspData(models.N1N2MESSAGETRANSFERCAUSE_N1_N2_TRANSFER_INITIATED)
				// context.StoreContextInDB(ue)
				return n1n2MessageTransferRspData, "", nil, nil
			case models.NGAPIETYPE_PDU_RES_REL_CMD:
				ue.ProducerLog.Debugln("AMF Transfer NGAP PDU Session Resource Release Command from SMF")
				list := ngapType.PDUSessionResourceToReleaseListRelCmd{}
				ngap_message.AppendPDUSessionResourceToReleaseListRelCmd(&list, smInfo.PduSessionId, n2Info)
				ngap_message.SendPDUSessionResourceReleaseCommand(ue.RanUe[anType], nasPdu, list)
				n1n2MessageTransferRspData = models.NewN1N2MessageTransferRspData(models.N1N2MESSAGETRANSFERCAUSE_N1_N2_TRANSFER_INITIATED)
				// context.StoreContextInDB(ue)
				return n1n2MessageTransferRspData, "", nil, nil
			default:
				ue.ProducerLog.Errorf("NGAP IE Type[%s] is not supported for SmInfo", smInfo.N2InfoContent.NgapIeType)
				problemDetails = utils.ProblemDetailsUnspecified()
				return nil, "", problemDetails, nil
			}
		}
	}

	// UE is CM-IDLE

	// 409: transfer a N2 PDU Session Resource Release Command to a 5G-AN and if the UE is in CM-IDLE
	if n2Info != nil && requestData.N2InfoContainer != nil && requestData.N2InfoContainer.SmInfo != nil &&
		requestData.N2InfoContainer.SmInfo.N2InfoContent.GetNgapIeType() == models.NGAPIETYPE_PDU_RES_REL_CMD {
		probDetails := utils.ProblemDetailsWithCause("UE in CM-IDLE state", http.StatusConflict, "UE is in CM-IDLE state", utils.CauseUeInCmIdleState)
		transferErr = models.NewN1N2MessageTransferError(*probDetails)
		return nil, "", nil, transferErr
	}
	// 504: the UE in MICO mode or the UE is only registered over Non-3GPP access and its state is CM-IDLE
	if !ue.State[models.ACCESSTYPE__3_GPP_ACCESS].Is(context.Registered) {
		probDetails := utils.ProblemDetailsWithCause("UE not reachable", http.StatusGatewayTimeout, "UE is not reachable", utils.CauseUeNotReachable)
		transferErr = models.NewN1N2MessageTransferError(*probDetails)
		return nil, "", nil, transferErr
	}

	n1n2MessageTransferRspData = models.NewN1N2MessageTransferRspDataWithDefaults()

	var pagingPriority *ngapType.PagingPriority

	var n1n2MessageID int64
	if n1n2MessageIDTmp, err := ue.N1N2MessageIDGenerator.Allocate(); err != nil {
		ue.ProducerLog.Errorf("allocate n1n2MessageID error: %+v", err)
		problemDetails = utils.ProblemDetailsSystemFailure(err.Error())
		return n1n2MessageTransferRspData, locationHeader, problemDetails, transferErr
	} else {
		n1n2MessageID = n1n2MessageIDTmp
	}
	locationHeader = context.AMF_Self().GetIPv4Uri() + reqUri + "/" + strconv.Itoa(int(n1n2MessageID))

	// Case A (UE is CM-IDLE in 3GPP access and the associated access type is 3GPP access)
	// in subclause 5.2.2.3.1.2 of TS29518
	if anType == models.ACCESSTYPE__3_GPP_ACCESS {
		if requestData.GetSkipInd() && n2Info == nil {
			n1n2MessageTransferRspData.Cause = models.N1N2MESSAGETRANSFERCAUSE_N1_MSG_NOT_TRANSFERRED
		} else {
			n1n2MessageTransferRspData.Cause = models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE
			message := context.N1N2Message{
				Request:     n1n2MessageTransferRequest,
				Status:      n1n2MessageTransferRspData.Cause,
				ResourceUri: locationHeader,
			}
			ue.N1N2Message = &message
			ue.SetOnGoing(anType, &context.OnGoingProcedureWithPrio{
				Procedure: context.OnGoingProcedurePaging,
				Ppi:       requestData.GetPpi(),
			})

			if onGoing.Ppi != 0 {
				pagingPriority = new(ngapType.PagingPriority)
				pagingPriority.Value = aper.Enumerated(onGoing.Ppi)
			}
			pkg, err := ngap_message.BuildPaging(ue, pagingPriority, false)
			if err != nil {
				logger.NgapLog.Errorf("build paging failed: %s", err.Error())
				// paging was never sent, so no T3513 will fire to clear the
				// transient state set above; reset it so the UE is not left
				// stuck in an OnGoingProcedurePaging conflict.
				ue.N1N2Message = nil
				ue.SetOnGoing(anType, &context.OnGoingProcedureWithPrio{
					Procedure: context.OnGoingProcedureNothing,
				})
				problemDetails = utils.ProblemDetailsSystemFailure(err.Error())
				return nil, "", problemDetails, nil
			}
			ngap_message.SendPaging(ue, pkg)
		}
		// TODO: WAITING_FOR_ASYNCHRONOUS_TRANSFER
		return n1n2MessageTransferRspData, locationHeader, nil, nil
	} else {
		// Case B (UE is CM-IDLE in Non-3GPP access but CM-CONNECTED in 3GPP access and the associated
		// access type is Non-3GPP access)in subclause 5.2.2.3.1.2 of TS29518
		if ue.CmConnect(models.ACCESSTYPE__3_GPP_ACCESS) {
			if n2Info == nil {
				n1n2MessageTransferRspData.Cause = models.N1N2MESSAGETRANSFERCAUSE_N1_N2_TRANSFER_INITIATED
				gmm_message.SendDLNASTransport(ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS], models.ACCESSTYPE__3_GPP_ACCESS,
					nasMessage.PayloadContainerTypeN1SMInfo, n1Msg, requestData.GetPduSessionId(), 0, nil, 0)
			} else {
				n1n2MessageTransferRspData.Cause = models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE
				message := context.N1N2Message{
					Request:     n1n2MessageTransferRequest,
					Status:      n1n2MessageTransferRspData.Cause,
					ResourceUri: locationHeader,
				}
				ue.N1N2Message = &message
				nasMsg, err := gmm_message.BuildNotification(ue, models.ACCESSTYPE_NON_3_GPP_ACCESS)
				if err != nil {
					logger.GmmLog.Errorf("build notification failed: %s", err.Error())
					// notification was never sent; clear the transient state and
					// surface the failure instead of replying 202 Accepted.
					ue.N1N2Message = nil
					problemDetails = utils.ProblemDetailsSystemFailure(err.Error())
					return nil, "", problemDetails, nil
				}
				gmm_message.SendNotification(ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS], nasMsg)
			}
			return n1n2MessageTransferRspData, locationHeader, nil, nil
		} else {
			// Case C ( UE is CM-IDLE in both Non-3GPP access and 3GPP access and the associated access ype is Non-3GPP access)
			// in subclause 5.2.2.3.1.2 of TS29518
			n1n2MessageTransferRspData.Cause = models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE
			message := context.N1N2Message{
				Request:     n1n2MessageTransferRequest,
				Status:      n1n2MessageTransferRspData.Cause,
				ResourceUri: locationHeader,
			}
			ue.N1N2Message = &message

			ue.SetOnGoing(anType, &context.OnGoingProcedureWithPrio{
				Procedure: context.OnGoingProcedurePaging,
				Ppi:       requestData.GetPpi(),
			})
			if onGoing.Ppi != 0 {
				pagingPriority = new(ngapType.PagingPriority)
				pagingPriority.Value = aper.Enumerated(onGoing.Ppi)
			}
			pkg, err := ngap_message.BuildPaging(ue, pagingPriority, true)
			if err != nil {
				logger.NgapLog.Errorf("build paging failed: %s", err.Error())
				// paging was never sent, so no T3513 will fire to clear the
				// transient state set above; reset it so the UE is not left
				// stuck in an OnGoingProcedurePaging conflict.
				ue.N1N2Message = nil
				ue.SetOnGoing(anType, &context.OnGoingProcedureWithPrio{
					Procedure: context.OnGoingProcedureNothing,
				})
				problemDetails = utils.ProblemDetailsSystemFailure(err.Error())
				return nil, "", problemDetails, nil
			}
			ngap_message.SendPaging(ue, pkg)
			return n1n2MessageTransferRspData, locationHeader, nil, nil
		}
	}
}

func HandleN1N2MessageTransferStatusRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.CommLog.Infoln("handle N1N2Message Transfer Status Request")

	ueContextID := request.Params["ueContextId"]
	reqUri := request.Params["reqUri"]

	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}

	if ue.EventChannel == nil {
		logger.CommLog.Warnln("EventChannel is nil for UE, invoking N1N2MessageTransferStatusProcedure directly")
		status, pd := N1N2MessageTransferStatusProcedure(ueContextID, reqUri)
		if pd != nil {
			return httpwrapper.NewResponse(int(pd.GetStatus()), nil, pd)
		}
		return httpwrapper.NewResponse(http.StatusOK, nil, &status)
	}

	sbiMsg := context.SbiMsg{
		UeContextId: ueContextID,
		ReqUri:      reqUri,
		Msg:         nil,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	ue.EventChannel.UpdateSbiHandler(ProducerHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result

	var n1n2MessageRspData *models.N1N2MessageTransferCause
	if msg.RespData != nil {
		n1n2MessageRspData = msg.RespData.(*models.N1N2MessageTransferCause)
	}

	// status, problemDetails := N1N2MessageTransferStatusProcedure(ueContextID, reqUri)
	if msg.ProblemDetails != nil {
		return httpwrapper.NewResponse(int(msg.ProblemDetails.(*models.ProblemDetails).GetStatus()), nil, msg.ProblemDetails.(*models.ProblemDetails))
	} else {
		return httpwrapper.NewResponse(http.StatusOK, nil, n1n2MessageRspData)
	}
}

func N1N2MessageTransferStatusProcedure(ueContextID string, reqUri string) (models.N1N2MessageTransferCause,
	*models.ProblemDetails,
) {
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return "", problemDetails
	}

	resourceUri := amfSelf.GetIPv4Uri() + reqUri
	n1n2Message := ue.N1N2Message
	if n1n2Message == nil || n1n2Message.ResourceUri != resourceUri {
		problemDetails := utils.ProblemDetailsContextNotFound("N1N2 message context not found")
		return "", problemDetails
	}

	return n1n2Message.Status, nil
}

// TS 29.518 5.2.2.3.3
func HandleN1N2MessageSubscirbeRequest(request *httpwrapper.Request) *httpwrapper.Response {
	ueN1N2InfoSubscriptionCreateData := request.Body.(models.UeN1N2InfoSubscriptionCreateData)
	ueContextID := request.Params["ueContextId"]

	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}

	if ue.EventChannel == nil {
		logger.CommLog.Warnln("EventChannel is nil for UE, invoking N1N2MessageSubscribeProcedure directly")
		createdData, pd := N1N2MessageSubscribeProcedure(ueContextID, ueN1N2InfoSubscriptionCreateData)
		if pd != nil {
			return httpwrapper.NewResponse(int(pd.GetStatus()), nil, pd)
		}
		return httpwrapper.NewResponse(http.StatusCreated, nil, createdData)
	}

	sbiMsg := context.SbiMsg{
		UeContextId: ueContextID,
		ReqUri:      "",
		Msg:         ueN1N2InfoSubscriptionCreateData,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	ue.EventChannel.UpdateSbiHandler(ProducerHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result

	var n1n2MessageRspData *models.UeN1N2InfoSubscriptionCreatedData
	if msg.RespData != nil {
		n1n2MessageRspData = msg.RespData.(*models.UeN1N2InfoSubscriptionCreatedData)
	}
	// ueN1N2InfoSubscriptionCreatedData, problemDetails := N1N2MessageSubscribeProcedure(ueContextID, ueN1N2InfoSubscriptionCreateData)
	if msg.ProblemDetails != nil {
		return httpwrapper.NewResponse(int(msg.ProblemDetails.(*models.ProblemDetails).GetStatus()), nil, msg.ProblemDetails.(*models.ProblemDetails))
	} else {
		return httpwrapper.NewResponse(http.StatusCreated, nil, n1n2MessageRspData)
	}
}

func N1N2MessageSubscribeProcedure(ueContextID string,
	ueN1N2InfoSubscriptionCreateData models.UeN1N2InfoSubscriptionCreateData) (
	*models.UeN1N2InfoSubscriptionCreatedData, *models.ProblemDetails,
) {
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return nil, problemDetails
	}

	ueN1N2InfoSubscriptionCreatedData := models.NewUeN1N2InfoSubscriptionCreatedDataWithDefaults()

	if newSubscriptionID, err := ue.N1N2MessageSubscribeIDGenerator.Allocate(); err != nil {
		logger.CommLog.Errorf("create subscriptionID Error: %+v", err)
		problemDetails := utils.ProblemDetailsSystemFailure(err.Error())
		return nil, problemDetails
	} else {
		ueN1N2InfoSubscriptionCreatedData.N1n2NotifySubscriptionId = strconv.Itoa(int(newSubscriptionID))
		ue.N1N2MessageSubscription.Store(newSubscriptionID, ueN1N2InfoSubscriptionCreateData)
	}
	return ueN1N2InfoSubscriptionCreatedData, nil
}

func HandleN1N2MessageUnSubscribeRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.CommLog.Infoln("handle N1N2Message Unsubscribe Request")

	ueContextID := request.Params["ueContextId"]
	subscriptionID := request.Params["subscriptionId"]

	problemDetails := N1N2MessageUnSubscribeProcedure(ueContextID, subscriptionID)
	if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	} else {
		return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}

func N1N2MessageUnSubscribeProcedure(ueContextID string, subscriptionID string) *models.ProblemDetails {
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return problemDetails
	}

	ue.N1N2MessageSubscription.Delete(subscriptionID)
	return nil
}
