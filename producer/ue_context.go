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
	"os"
	"strings"

	"github.com/omec-project/amf/consumer"
	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
)

func createTempBinaryFile(data []byte) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", "prefix")
	if err != nil {
		return nil, err
	}
	if _, err = tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, err
	}
	if _, err = tmpFile.Seek(0, io.SeekStart); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, err
	}
	return tmpFile, nil
}

func UeContextHandler(ctx ctxt.Context, s1, s2 string, msg interface{}) (interface{}, string, interface{}, interface{}) {
	switch msg := msg.(type) {
	case models.CreateUEContextRequest:
		r1, r2 := createUEContextProcedure(s1, msg)
		return r1, "", nil, r2
	case models.UEContextRelease:
		r1 := releaseUEContextProcedure(s1, msg)
		return nil, "", r1, nil
	case models.UEContextTransferRequest:
		r1, r2 := ueContextTransferProcedure(s1, msg)
		return r1, "", r2, nil
	case models.AssignEbiData:
		r1, r2, r3 := assignEbiDataProcedure(s1, msg)
		return r1, "", r3, r2
	case models.UeRegStatusUpdateReqData:
		r1, r2 := registrationStatusUpdateProcedure(ctx, s1, msg)
		return r1, "", r2, nil
	}

	return nil, "", nil, nil
}

// TS 29.518 5.2.2.2.3
func HandleCreateUEContextRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.CommLog.Infof("Handle Create UE Context Request")

	createUeContextRequest := request.Body.(models.CreateUEContextRequest)
	ueContextID := request.Params["ueContextId"]

	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	sbiMsg := context.SbiMsg{
		UeContextId: ueContextID,
		ReqUri:      "",
		Msg:         createUeContextRequest,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	var createUeContextRspData *models.CreateUEContext201Response
	var ueContextCreateErr *models.UeContextCreateError
	ue.EventChannel.UpdateSbiHandler(UeContextHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result
	if msg.RespData != nil {
		createUeContextRspData = msg.RespData.(*models.CreateUEContext201Response)
	}
	if msg.TransferErr != nil {
		ueContextCreateErr = msg.TransferErr.(*models.UeContextCreateError)
	}
	// createUeContextResponse, ueContextCreateError := createUEContextProcedure(ueContextID, createUeContextRequest)
	if ueContextCreateErr != nil {
		return httpwrapper.NewResponse(int(ueContextCreateErr.Error.GetStatus()), nil, ueContextCreateErr)
	} else {
		return httpwrapper.NewResponse(http.StatusCreated, nil, createUeContextRspData)
	}
}

func createUEContextProcedure(ueContextID string, createUeContextRequest models.CreateUEContextRequest) (
	*models.CreateUEContext201Response, *models.UeContextCreateError,
) {
	amfSelf := context.AMF_Self()
	ueContextCreateData, ok := createUeContextRequest.GetJsonDataOk()

	if !ok {
		problemDetails := utils.ProblemDetailsWithCause("Handover failure", http.StatusForbidden, "Failed to get JSON data", utils.CauseHandoverFailure)
		ueContextCreateError := models.NewUeContextCreateError(*problemDetails)
		return nil, ueContextCreateError
	}
	// create the UE context in target amf
	ue := amfSelf.NewAmfUe(ueContextID)
	// amfSelf.AmfRanSetByRanId(*ueContextCreateData.TargetId.RanNodeId)
	// ue.N1N2Message[ueContextId] = &context.N1N2Message{}
	// ue.N1N2Message[ueContextId].Request.JsonData = &models.N1N2MessageTransferReqData{
	// 	N2InfoContainer: &models.N2InfoContainer{
	// 		SmInfo: &models.N2SmInformation{
	// 			N2InfoContent: ueContextCreateData.SourceToTargetData,
	// 		},
	// 	},
	// }
	ue.HandoverNotifyUri = ueContextCreateData.GetN2NotifyUri()

	amfSelf.AmfRanFindByRanID(*ueContextCreateData.TargetId.RanNodeId.Get())
	supportedTAI := context.NewSupportedTAI()
	supportedTAI.Tai.Tac = ueContextCreateData.TargetId.Tai.Tac
	supportedTAI.Tai.PlmnId = ueContextCreateData.TargetId.Tai.PlmnId
	// ue.N1N2MessageSubscribeInfo[ueContextID] = &models.UeN1N2InfoSubscriptionCreateData{
	// 	N2NotifyCallbackUri: ueContextCreateData.N2NotifyUri,
	// }
	ue.UnauthenticatedSupi = ueContextCreateData.UeContext.GetSupiUnauthInd()
	// should be smInfo list

	//for _, smInfo := range ueContextCreateData.PduSessionList {
	//if smInfo.N2InfoContent.NgapIeType == "NGAPIETYPE_HANDOVER_REQUIRED" {
	// ue.N1N2Message[amfSelf.Uri].Request.JsonData.N2InfoContainer.SmInfo = &smInfo
	//}
	//}

	ue.RoutingIndicator = ueContextCreateData.UeContext.GetRoutingIndicator()

	// optional
	ue.UdmGroupId = ueContextCreateData.UeContext.GetUdmGroupId()
	ue.AusfGroupId = ueContextCreateData.UeContext.GetAusfGroupId()
	// ueContextCreateData.UeContext.HpcfId
	// RestrictedRatList contains RAT types the UE cannot use, so it must
	// not be copied into ue.RatType (semantically inverted, and indexing
	// [0] can panic on an empty list — minItem = -1 per spec). RatType is
	// set from the access type during HandleRegistrationRequest
	// (gmm/handler.go) and upgraded for NTN cells based on RATInformation
	// from NGSetup.
	// ueContextCreateData.UeContext.ForbiddenAreaList
	// ueContextCreateData.UeContext.ServiceAreaRestriction
	// ueContextCreateData.UeContext.RestrictedCoreNwTypeList

	// it's not in 5.2.2.1.1 step 2a, so don't support
	// ue.Gpsi = ueContextCreateData.UeContext.GpsiList
	// ue.Pei = ueContextCreateData.UeContext.Pei
	// ueContextCreateData.UeContext.GroupList
	// ueContextCreateData.UeContext.DrxParameter
	// ueContextCreateData.UeContext.SubRfsp
	// ueContextCreateData.UeContext.UsedRfsp
	// ue.UEAMBR = ueContextCreateData.UeContext.SubUeAmbr
	// ueContextCreateData.UeContext.SmsSupport
	// ueContextCreateData.UeContext.SmsfId
	// ueContextCreateData.UeContext.SeafData
	// ueContextCreateData.UeContext.Var5gMmCapability
	// ueContextCreateData.UeContext.PcfId
	// ueContextCreateData.UeContext.PcfAmPolicyUri
	// ueContextCreateData.UeContext.AmPolicyReqTriggerList
	// ueContextCreateData.UeContext.EventSubscriptionList
	// ueContextCreateData.UeContext.MmContextList
	// ue.CurPduSession.PduSessionId = ueContextCreateData.UeContext.SessionContextList.
	// ue.TraceData = ueContextCreateData.UeContext.TraceData
	createUeContextResponse := models.NewCreateUEContext201Response()
	ueContextCreatedData := models.UeContextCreatedData{
		UeContext: models.UeContext{
			Supi: ueContextCreateData.UeContext.Supi,
		},
		PduSessionList:   ueContextCreateData.PduSessionList,
		PcfReselectedInd: openapi.PtrBool(false),
	}
	createUeContextResponse.SetJsonData(ueContextCreatedData)

	// TODO: When  Target AMF selects a nw PCF for AM policy, set the flag to true.

	//response.UeContext = ueContextCreateData.UeContext
	//response.TargetToSourceData = ue.N1N2Message[amfSelf.Uri].Request.JsonData.N2InfoContainer.SmInfo.N2InfoContent
	//response.PduSessionList = ueContextCreateData.PduSessionList
	//response.PcfReselectedInd = false // TODO:When  Target AMF selects a nw PCF for AM policy, set the flag to true.
	//

	// return httpwrapper.NewResponse(http.StatusCreated, nil, createUeContextResponse)
	return createUeContextResponse, nil
}

// TS 29.518 5.2.2.2.4
func HandleReleaseUEContextRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.CommLog.Info("Handle Release UE Context Request")

	ueContextRelease := request.Body.(models.UEContextRelease)
	ueContextID := request.Params["ueContextId"]
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	sbiMsg := context.SbiMsg{
		UeContextId: ueContextID,
		ReqUri:      "",
		Msg:         ueContextRelease,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	ue.EventChannel.UpdateSbiHandler(UeContextHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result

	// problemDetails := releaseUEContextProcedure(ueContextID, ueContextRelease)
	if msg.ProblemDetails != nil {
		return httpwrapper.NewResponse(int(msg.ProblemDetails.(*models.ProblemDetails).GetStatus()), nil, msg.ProblemDetails.(*models.ProblemDetails))
	} else {
		return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}

func releaseUEContextProcedure(ueContextID string, ueContextRelease models.UEContextRelease) *models.ProblemDetails {
	amfSelf := context.AMF_Self()

	// TODO: UE is emergency registered and the SUPI is not authenticated
	if ueContextRelease.GetSupi() != "" {
		logger.GmmLog.Warnf("Emergency registered UE is not supported.")
		problemDetails := utils.ProblemDetailsUnspecified()
		return problemDetails
	}

	if _, ok := ueContextRelease.GetNgapCauseOk(); !ok {
		problemDetails := utils.ProblemDetailsMandatoryIeMissing("NgapCause is missing")
		return problemDetails
	}

	logger.CommLog.Debugf("Release UE Context NGAP cause: %+v", ueContextRelease.NgapCause)

	if ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID); ok {
		ue.Remove()
	} else {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return problemDetails
	}

	return nil
}

// TS 29.518 5.2.2.2.1
func HandleUEContextTransferRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.CommLog.Info("Handle UE Context Transfer Request")

	ueContextTransferRequest := request.Body.(models.UEContextTransferRequest)
	ueContextID := request.Params["ueContextId"]

	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	sbiMsg := context.SbiMsg{
		UeContextId: ueContextID,
		ReqUri:      "",
		Msg:         ueContextTransferRequest,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	var ueContextTransferResponse *models.UEContextTransfer200Response
	ue.EventChannel.UpdateSbiHandler(UeContextHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result
	if msg.RespData != nil {
		ueContextTransferResponse = msg.RespData.(*models.UEContextTransfer200Response)
	}

	// ueContextTransferResponse, problemDetails := ueContextTransferProcedure(ueContextID, ueContextTransferRequest)
	if msg.ProblemDetails != nil {
		return httpwrapper.NewResponse(int(msg.ProblemDetails.(*models.ProblemDetails).GetStatus()), nil, msg.ProblemDetails.(*models.ProblemDetails))
	} else {
		return httpwrapper.NewResponse(http.StatusOK, nil, ueContextTransferResponse)
	}
}

func ueContextTransferProcedure(ueContextID string, ueContextTransferRequest models.UEContextTransferRequest) (
	*models.UEContextTransfer200Response, *models.ProblemDetails,
) {
	amfSelf := context.AMF_Self()

	if ueContextTransferRequest.JsonData == nil {
		problemDetails := utils.ProblemDetailsMandatoryIeMissing("JsonData is missing")
		return nil, problemDetails
	}

	ueContextTransferReqData := ueContextTransferRequest.GetJsonData()

	if ueContextTransferReqData.GetAccessType() == "" || ueContextTransferReqData.GetReason() == "" {
		problemDetails := utils.ProblemDetailsMandatoryIeMissing("AccessType or Reason is missing")
		return nil, problemDetails
	}

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return nil, problemDetails
	}

	ueContextTransferResponse := models.NewUEContextTransfer200Response()
	ueContextTransferResponse.SetJsonData(models.UeContextTransferRspData{})
	ueContextTransferRspData := ueContextTransferResponse.JsonData

	//if ue.GetAnType() != UeContextTransferReqData.AccessType {
	//for _, tai := range ue.RegistrationArea[ue.GetAnType()] {
	//if UeContextTransferReqData.PlmnId == tai.PlmnId {
	// TODO : generate N2 signalling
	//}
	//}
	//}

	switch ueContextTransferReqData.GetReason() {
	case models.TRANSFERREASON_INIT_REG:
		// TODO: check integrity of the registration request included in ueContextTransferRequest
		// TODO: handle condition of TS 29.518 5.2.2.2.1.1 step 2a case b
		ueContextTransferRspData.SetUeContext(buildUEContextModel(ue))
	case models.TRANSFERREASON_MOBI_REG:
		// TODO: check integrity of the registration request included in ueContextTransferRequest
		ueContextTransferRspData.SetUeContext(buildUEContextModel(ue))

		sessionContextList := &ueContextTransferRspData.UeContext.SessionContextList
		ue.SmContextList.Range(func(key, value interface{}) bool {
			smContext := value.(*context.SmContext)
			snssai := smContext.Snssai()
			pduSessionContext := models.PduSessionContext{
				PduSessionId: smContext.PduSessionID(),
				SmContextRef: smContext.SmContextRef(),
				SNssai:       snssai,
				Dnn:          smContext.Dnn(),
				AccessType:   smContext.AccessType(),
				HsmfId:       openapi.PtrString(smContext.HSmfID()),
				VsmfId:       openapi.PtrString(smContext.VSmfID()),
				NsInstance:   openapi.PtrString(smContext.NsInstance()),
			}
			*sessionContextList = append(*sessionContextList, pduSessionContext)
			return true
		})

		ueContextTransferRspData.SetUeRadioCapability(models.N2InfoContent{
			NgapMessageType: openapi.PtrInt32(0),
			NgapIeType:      models.NGAPIETYPE_UE_RADIO_CAPABILITY.Ptr(),
			NgapData: models.RefToBinaryData{
				ContentId: "n2Info",
			},
		})
		tmpFile, err := createTempBinaryFile([]byte(ue.UeRadioCapability))
		if err != nil {
			logger.ProducerLog.Errorf("create binaryDataN2Information failed: %+v", err)
			problemDetails := utils.ProblemDetailsSystemFailure(err.Error())
			return nil, problemDetails
		}
		ueContextTransferResponse.BinaryDataN2Information = &tmpFile
	case models.TRANSFERREASON_MOBI_REG_UE_VALIDATED:
		ueContextTransferRspData.SetUeContext(buildUEContextModel(ue))

		sessionContextList := &ueContextTransferRspData.UeContext.SessionContextList
		ue.SmContextList.Range(func(key, value interface{}) bool {
			smContext := value.(*context.SmContext)
			snssai := smContext.Snssai()
			pduSessionContext := models.PduSessionContext{
				PduSessionId: smContext.PduSessionID(),
				SmContextRef: smContext.SmContextRef(),
				SNssai:       snssai,
				Dnn:          smContext.Dnn(),
				AccessType:   smContext.AccessType(),
				HsmfId:       openapi.PtrString(smContext.HSmfID()),
				VsmfId:       openapi.PtrString(smContext.VSmfID()),
				NsInstance:   openapi.PtrString(smContext.NsInstance()),
			}
			*sessionContextList = append(*sessionContextList, pduSessionContext)
			return true
		})

		ueRadioCapability := models.NewN2InfoContent(models.RefToBinaryData{
			ContentId: "n2Info",
		})
		ueRadioCapability.SetNgapMessageType(0)
		ueRadioCapability.SetNgapIeType(models.NGAPIETYPE_UE_RADIO_CAPABILITY)
		ueContextTransferRspData.UeRadioCapability = ueRadioCapability
		tmpFile, err := createTempBinaryFile([]byte(ue.UeRadioCapability))
		if err != nil {
			logger.ProducerLog.Errorf("create binaryDataN2Information failed: %+v", err)
			problemDetails := utils.ProblemDetailsSystemFailure(err.Error())
			return nil, problemDetails
		}
		ueContextTransferResponse.BinaryDataN2Information = &tmpFile
	default:
		logger.ProducerLog.Warnf("Invalid Transfer Reason: %+v", ueContextTransferReqData.GetReason())
		problemDetails := utils.ProblemDetailsWithInvalidParams(
			"Mandatory IE incorrect",
			http.StatusForbidden,
			"Invalid transfer reason",
			[]models.InvalidParam{
				{
					Param: "reason",
				},
			},
		)
		problemDetails.SetCause(utils.CauseMandatoryIeIncorrect)
		return nil, problemDetails
	}
	return ueContextTransferResponse, nil
}

func buildUEContextModel(ue *context.AmfUe) models.UeContext {
	ueContext := models.NewUeContext()
	ueContext.SetSupi(ue.GetSupi())
	ueContext.SetSupiUnauthInd(ue.UnauthenticatedSupi)

	if gpsi := ue.GetGpsi(); gpsi != "" {
		ueContext.GpsiList = append(ueContext.GpsiList, gpsi)
	}

	if pei := ue.GetPei(); pei != "" {
		ueContext.SetPei(pei)
	}

	if ue.UdmGroupId != "" {
		ueContext.SetUdmGroupId(ue.UdmGroupId)
	}

	if ue.AusfGroupId != "" {
		ueContext.SetAusfGroupId(ue.AusfGroupId)
	}

	if ue.RoutingIndicator != "" {
		ueContext.SetRoutingIndicator(ue.RoutingIndicator)
	}

	if ue.AccessAndMobilitySubscriptionData != nil {
		if ue.AccessAndMobilitySubscriptionData.HasSubscribedUeAmbr() {
			ueContext.SubUeAmbr = models.NewAmbr(ue.AccessAndMobilitySubscriptionData.SubscribedUeAmbr.GetUplink(), ue.AccessAndMobilitySubscriptionData.SubscribedUeAmbr.GetDownlink())
		}
		if rfspIndex, ok := ue.AccessAndMobilitySubscriptionData.GetRfspIndexOk(); ok && rfspIndex != nil && *rfspIndex != 0 {
			ueContext.SetSubRfsp(*rfspIndex)
		}
	}

	if ue.PcfId != "" {
		ueContext.SetPcfId(ue.PcfId)
	}

	if ue.AmPolicyUri != "" {
		ueContext.SetPcfAmPolicyUri(ue.AmPolicyUri)
	}

	if ue.AmPolicyAssociation != nil {
		if len(ue.AmPolicyAssociation.Triggers) > 0 {
			ueContext.AmPolicyReqTriggerList = buildAmPolicyReqTriggers(ue.AmPolicyAssociation.Triggers)
		}
	}

	for _, eventSub := range ue.EventSubscriptionsInfo {
		if eventSub.EventSubscription != nil {
			ueContext.EventSubscriptionList = append(ueContext.EventSubscriptionList, *eventSub.EventSubscription)
		}
	}

	if ue.TraceData != nil {
		ueContext.SetTraceData(*ue.TraceData)
	}
	return *ueContext
}

func buildAmPolicyReqTriggers(triggers []models.RequestTrigger) (amPolicyReqTriggers []models.PolicyReqTrigger) {
	for _, trigger := range triggers {
		switch trigger {
		case models.REQUESTTRIGGER_LOC_CH:
			amPolicyReqTriggers = append(amPolicyReqTriggers, models.POLICYREQTRIGGER_LOCATION_CHANGE)
		case models.REQUESTTRIGGER_PRA_CH:
			amPolicyReqTriggers = append(amPolicyReqTriggers, models.POLICYREQTRIGGER_PRA_CHANGE)
			// TODO: GA: POLICYREQTRIGGER_SARI_CHANGE and POLICYREQTRIGGER_RFSP_INDEX_CHANGE not implemented in context package
			// case models.REQUESTTRIGGER_SERV_AREA_CH:
			// 	amPolicyReqTriggers = append(amPolicyReqTriggers, models.POLICYREQTRIGGER_SARI_CHANGE)
			// case models.REQUESTTRIGGER_RFSP_CH:
			// 	amPolicyReqTriggers = append(amPolicyReqTriggers, models.POLICYREQTRIGGER_RFSP_INDEX_CHANGE)
		}
	}
	return
}

// TS 29.518 5.2.2.6
func HandleAssignEbiDataRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.CommLog.Info("Handle Assign Ebi Data Request")

	assignEbiData := request.Body.(models.AssignEbiData)
	ueContextID := request.Params["ueContextId"]

	amfSelf := context.AMF_Self()

	// assignedEbiData, assignEbiError, problemDetails := AssignEbiDataProcedure(ueContextID, assignEbiData)
	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	sbiMsg := context.SbiMsg{
		UeContextId: ueContextID,
		ReqUri:      "",
		Msg:         assignEbiData,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	var assignEbiRspData *models.AssignedEbiData
	var assignEbiErr *models.AssignEbiError
	ue.EventChannel.UpdateSbiHandler(UeContextHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result
	if msg.RespData != nil {
		assignEbiRspData = msg.RespData.(*models.AssignedEbiData)
	}
	if msg.TransferErr != nil {
		assignEbiErr = msg.TransferErr.(*models.AssignEbiError)
	}

	if msg.ProblemDetails != nil {
		return httpwrapper.NewResponse(int(msg.ProblemDetails.(*models.ProblemDetails).GetStatus()), nil, msg.ProblemDetails.(*models.ProblemDetails))
	} else if assignEbiErr != nil {
		return httpwrapper.NewResponse(int(assignEbiErr.Error.GetStatus()), nil, assignEbiErr)
	} else {
		return httpwrapper.NewResponse(http.StatusOK, nil, assignEbiRspData)
	}
}

func assignEbiDataProcedure(ueContextID string, assignEbiData models.AssignEbiData) (
	*models.AssignedEbiData, *models.AssignEbiError, *models.ProblemDetails,
) {
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return nil, nil, problemDetails
	}

	// TODO: AssignEbiError not used, check it!
	if _, ok := ue.SmContextFindByPDUSessionID(assignEbiData.GetPduSessionId()); ok {
		assignedEbiData := models.NewAssignedEbiDataWithDefaults()
		assignedEbiData.SetPduSessionId(assignEbiData.GetPduSessionId())
		return assignedEbiData, nil, nil
	} else {
		logger.ProducerLog.Errorf("no SM context found for PDU session ID %d", assignEbiData.GetPduSessionId())
		return nil, nil, nil
	}
}

// HandleRegistrationStatusUpdateRequest TS 29.518 5.2.2.2.2
func HandleRegistrationStatusUpdateRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.CommLog.Info("Handle Registration Status Update Request")

	ueRegStatusUpdateReqData, ok := request.Body.(models.UeRegStatusUpdateReqData)
	if !ok {
		problemDetails := utils.ProblemDetailsWithCause("Invalid body format", http.StatusBadRequest, "Request body format is invalid", utils.CauseInvalidBodyFormat)
		return httpwrapper.NewResponse(http.StatusBadRequest, nil, problemDetails)
	}
	ueContextID := request.Params["ueContextId"]

	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return httpwrapper.NewResponse(http.StatusNotFound, nil, problemDetails)
	}
	sbiMsg := context.SbiMsg{
		UeContextId: ueContextID,
		ReqUri:      "",
		Msg:         ueRegStatusUpdateReqData,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	var ueRegStatusUpdateRspData *models.UeRegStatusUpdateRspData
	ue.EventChannel.UpdateSbiHandler(UeContextHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg, read := <-sbiMsg.Result
	if !read {
		problemDetails := utils.ProblemDetailsWithCause("Message not received", http.StatusInternalServerError, "Message not received from channel", utils.CauseMessageNotReceived)
		return httpwrapper.NewResponse(http.StatusInternalServerError, nil, problemDetails)
	}
	ueRegStatusUpdateRspData, ok = msg.RespData.(*models.UeRegStatusUpdateRspData)
	if !ok {
		if msg.ProblemDetails != nil {
			if problemDetails, ok := msg.ProblemDetails.(*models.ProblemDetails); ok {
				return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
			}
		}
		// Handle unexpected response data type
		problemDetails := utils.ProblemDetailsWithCause("Unexpected response type", http.StatusInternalServerError, "Unexpected response data type", utils.CauseUnexpectedResponseType)
		return httpwrapper.NewResponse(http.StatusInternalServerError, nil, problemDetails)
	}
	return httpwrapper.NewResponse(http.StatusOK, nil, ueRegStatusUpdateRspData)
}

func registrationStatusUpdateProcedure(ctx ctxt.Context, ueContextID string, ueRegStatusUpdateReqData models.UeRegStatusUpdateReqData) (
	*models.UeRegStatusUpdateRspData, *models.ProblemDetails,
) {
	amfSelf := context.AMF_Self()

	// ueContextID must be a 5g GUTI (TS 29.518 6.1.3.2.4.5.1)
	if !strings.HasPrefix(ueContextID, "5g-guti") {
		problemDetails := utils.ProblemDetailsUnspecified()
		return nil, problemDetails
	}

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
		return nil, problemDetails
	}

	ueRegStatusUpdateRspData := new(models.UeRegStatusUpdateRspData)

	if ueRegStatusUpdateReqData.TransferStatus == models.UECONTEXTTRANSFERSTATUS_TRANSFERRED {
		// remove the individual ueContext resource and release any PDU session(s)
		for _, pduSessionId := range ueRegStatusUpdateReqData.ToReleaseSessionList {
			cause := models.CAUSE_REL_DUE_TO_SLICE_NOT_AVAILABLE
			causeAll := &context.CauseAll{
				Cause: &cause,
			}
			smContext, ok := ue.SmContextFindByPDUSessionID(pduSessionId)
			if !ok {
				ue.ProducerLog.Errorf("SmContext[PDU Session ID:%d] not found", pduSessionId)
			}
			problem, err := consumer.SendReleaseSmContextRequest(ue, smContext, causeAll, "", nil)
			if problem != nil {
				logger.GmmLog.Errorf("Release SmContext[pduSessionId: %d] Failed Problem[%+v]", pduSessionId, problem)
			} else if err != nil {
				logger.GmmLog.Errorf("Release SmContext[pduSessionId: %d] Error[%v]", pduSessionId, err.Error())
			}
		}

		if ueRegStatusUpdateReqData.GetPcfReselectedInd() {
			problem, err := consumer.AMPolicyControlDelete(ctx, ue)
			if problem != nil {
				logger.GmmLog.Errorf("AM Policy Control Delete Failed Problem[%+v]", problem)
			} else if err != nil {
				logger.GmmLog.Errorf("AM Policy Control Delete Error[%v]", err.Error())
			}
		}

		ue.Remove()
	} else {
		// NOT_TRANSFERRED
		logger.CommLog.Debug("[AMF] RegistrationStatusUpdate: NOT_TRANSFERRED")
	}

	ueRegStatusUpdateRspData.RegStatusTransferComplete = true
	return ueRegStatusUpdateRspData, nil
}
