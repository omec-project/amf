// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Namf_Communication"
	"github.com/omec-project/openapi/v2/models"
	"go.opentelemetry.io/otel/attribute"
)

func BuildUeContextModel(ue *amf_context.AmfUe) (ueContext models.UeContext) {
	ueContext.Supi = openapi.PtrString(ue.GetSupi())
	ueContext.SupiUnauthInd = openapi.PtrBool(ue.UnauthenticatedSupi)

	if ue.GetGpsi() != "" {
		ueContext.GpsiList = append(ueContext.GpsiList, ue.GetGpsi())
	}

	if ue.GetPei() != "" {
		ueContext.Pei = openapi.PtrString(ue.GetPei())
	}

	if ue.UdmGroupId != "" {
		ueContext.UdmGroupId = openapi.PtrString(ue.UdmGroupId)
	}

	if ue.AusfGroupId != "" {
		ueContext.AusfGroupId = openapi.PtrString(ue.AusfGroupId)
	}

	if ue.RoutingIndicator != "" {
		ueContext.RoutingIndicator = openapi.PtrString(ue.RoutingIndicator)
	}

	if ue.AccessAndMobilitySubscriptionData != nil {
		if ambr, ok := ue.AccessAndMobilitySubscriptionData.GetSubscribedUeAmbrOk(); ok {
			ueContext.SubUeAmbr = models.NewAmbr(ambr.GetUplink(), ambr.GetDownlink())
		}
		if ue.AccessAndMobilitySubscriptionData.GetRfspIndex() != 0 {
			ueContext.SubRfsp = openapi.PtrInt32(ue.AccessAndMobilitySubscriptionData.GetRfspIndex())
		}
	}

	if ue.PcfId != "" {
		ueContext.PcfId = openapi.PtrString(ue.PcfId)
	}

	if ue.AmPolicyUri != "" {
		ueContext.PcfAmPolicyUri = openapi.PtrString(ue.AmPolicyUri)
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
		traceData := models.NewNullableTraceData(ue.TraceData)
		ueContext.TraceData = *traceData
	}
	return ueContext
}

func buildAmPolicyReqTriggers(triggers []models.RequestTrigger) (amPolicyReqTriggers []models.PolicyReqTrigger) {
	for _, trigger := range triggers {
		switch trigger {
		case models.REQUESTTRIGGER_LOC_CH:
			amPolicyReqTriggers = append(amPolicyReqTriggers, models.POLICYREQTRIGGER_LOCATION_CHANGE)
		case models.REQUESTTRIGGER_PRA_CH:
			amPolicyReqTriggers = append(amPolicyReqTriggers, models.POLICYREQTRIGGER_PRA_CHANGE)
		// case models.REQUESTTRIGGER_SERV_AREA_CH:
		// 	amPolicyReqTriggers = append(amPolicyReqTriggers, models.POLICYREQTRIGGER_SARI_CHANGE)
		// case models.REQUESTTRIGGER_RFSP_CH:
		// 	amPolicyReqTriggers = append(amPolicyReqTriggers, models.POLICYREQTRIGGER_RFSP_INDEX_CHANGE)
		// TODO: GA: Review the above two policies that were removed in Rel-18
		default:
			logger.ConsumerLog.Errorf("ignoring unknown policy trigger: %v", trigger)
			continue
		}
	}
	return
}

func UEContextTransferRequest(
	ctx context.Context, ue *amf_context.AmfUe, accessType models.AccessType, transferReason models.TransferReason) (
	ueContextTransferRspData *models.UeContextTransferRspData, problemDetails *models.ProblemDetails, err error,
) {
	ctx, span := tracer.Start(ctx, "HTTP POST amf/ue-contexts/{ueContextId}/transfer")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "amf"),
		attribute.String("net.peer.name", ue.TargetAmfUri),
		attribute.String("ue.supi", ue.GetSupi()),
		attribute.String("ue.plmn.id", ue.PlmnId.GetMcc()+ue.PlmnId.GetMnc()),
	)

	ueContextTransferReqData := models.UeContextTransferReqData{
		Reason:     transferReason,
		AccessType: accessType,
	}
	var regRequestFile *os.File

	if transferReason == models.TRANSFERREASON_INIT_REG || transferReason == models.TRANSFERREASON_MOBI_REG {
		var buf bytes.Buffer
		ue.RegistrationRequest.EncodeRegistrationRequest(&buf)

		regRequestFile, err = createBinaryPayloadTempFile(buf.Bytes())
		if err != nil {
			return ueContextTransferRspData, problemDetails, err
		}
		if regRequestFile != nil {
			defer os.Remove(regRequestFile.Name())
			defer regRequestFile.Close()
		}

		ueContextTransferReqData.RegRequest = models.NewN1MessageContainer(models.N1MESSAGECLASS__5_GMM, models.RefToBinaryData{
			ContentId: "n1Msg",
		})
	}

	// guti format is defined at TS 29.518 Table 6.1.3.2.2-1 5g-guti-[0-9]{5,6}[0-9a-fA-F]{14}
	ueContextId := fmt.Sprintf("5g-guti-%s", ue.GetGuti())

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	requestURI := fmt.Sprintf("%s/namf-comm/v1/ue-contexts/%s/transfer",
		strings.TrimRight(ue.TargetAmfUri, "/"), url.PathEscape(ueContextId))
	requestBody := &bytes.Buffer{}
	contentType := "application/json"
	if regRequestFile != nil {
		ueContextTransferRequest := models.NewUEContextTransferRequest()
		ueContextTransferRequest.SetJsonData(ueContextTransferReqData)
		ueContextTransferRequest.SetBinaryDataN1Message(regRequestFile)
		contentType, err = openapi.MultipartEncode(ueContextTransferRequest, requestBody)
	} else {
		requestBody, err = openapi.SetBody(ueContextTransferReqData, contentType)
	}
	if err != nil {
		return ueContextTransferRspData, problemDetails, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURI, bytes.NewReader(requestBody.Bytes()))
	if err != nil {
		return ueContextTransferRspData, problemDetails, err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("Accept", "application/json, multipart/related, application/problem+json")

	httpResp, localErr := http.DefaultClient.Do(req)
	if localErr != nil {
		err = openapi.ReportError("%s: server no response", ue.TargetAmfUri)
		return ueContextTransferRspData, problemDetails, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode < http.StatusMultipleChoices {
		ueContextTransferResponse := models.NewUEContextTransfer200Response()
		if err = decodeSuccessResponseBody(httpResp, ueContextTransferResponse); err != nil {
			return ueContextTransferRspData, problemDetails, err
		}
		if ueContextTransferResponse.JsonData != nil {
			ueContextTransferRspData = ueContextTransferResponse.JsonData
		}
		logger.ConsumerLog.Debugf("UeContextTransferRspData: %+v", ueContextTransferRspData)
		return ueContextTransferRspData, problemDetails, nil
	}

	problemDetails = models.NewProblemDetails()
	if err = decodeSuccessResponseBody(httpResp, problemDetails); err != nil {
		return ueContextTransferRspData, nil, err
	}
	return ueContextTransferRspData, problemDetails, err
}

// This operation is called "RegistrationCompleteNotify" at TS 23.502
func RegistrationStatusUpdate(ctx context.Context, ue *amf_context.AmfUe, request models.UeRegStatusUpdateReqData) (
	regStatusTransferComplete bool, problemDetails *models.ProblemDetails, err error,
) {
	ctx, span := tracer.Start(ctx, "HTTP POST amf/ue-contexts/{ueContextId}/transfer-update")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "amf"),
		attribute.String("net.peer.name", ue.TargetAmfUri),
		attribute.String("ue.supi", ue.GetSupi()),
		attribute.String("ue.plmn.id", ue.PlmnId.GetMcc()+ue.PlmnId.GetMnc()),
	)

	configuration := Namf_Communication.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.TargetAmfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Namf_Communication.NewAPIClient(configuration)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ueContextId := fmt.Sprintf("5g-guti-%s", ue.GetGuti())
	apiRegistrationStatusUpdateRequest := client.IndividualUeContextDocumentAPI.RegistrationStatusUpdate(ctx, ueContextId)
	apiRegistrationStatusUpdateRequest = apiRegistrationStatusUpdateRequest.UeRegStatusUpdateReqData(request)
	res, httpResp, localErr := client.IndividualUeContextDocumentAPI.RegistrationStatusUpdateExecute(apiRegistrationStatusUpdateRequest)
	if localErr == nil {
		regStatusTransferComplete = res.RegStatusTransferComplete
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			err = localErr
			return regStatusTransferComplete, problemDetails, err
		}
		if problem, ok := openapi.ErrorModel[models.ProblemDetails](localErr); ok {
			problemDetails = &problem
		} else {
			err = localErr
		}
	} else {
		err = openapi.ReportError("%s: server no response", ue.TargetAmfUri)
	}
	return regStatusTransferComplete, problemDetails, err
}
