// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package producer

import (
	"net/http"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/http_wrapper"
	"github.com/omec-project/openapi/models"
)

func MtHandler(s1, s2 string, msg interface{}) (interface{}, string, interface{}, interface{}) {
	switch msg.(type) {
	case string:
		r1, r2 := ProvideDomainSelectionInfoProcedure(s1, s2, msg.(string))
		return r1, "", r2, nil
	}

	return nil, "", nil, nil
}

func HandleProvideDomainSelectionInfoRequest(request *http_wrapper.Request) *http_wrapper.Response {
	var ue *context.AmfUe
	var ok bool
	logger.MtLog.Info("Handle Provide Domain Selection Info Request")

	ueContextID := request.Params["ueContextId"]
	infoClassQuery := request.Query.Get("info-class")
	supportedFeaturesQuery := request.Query.Get("supported-features")

	amfSelf := context.AMF_Self()

	if ue, ok = amfSelf.AmfUeFindByUeContextID(ueContextID); !ok {
		problemDetails := &models.ProblemDetails{
			Status: http.StatusNotFound,
			Cause:  "CONTEXT_NOT_FOUND",
		}
		return http_wrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
	}
	sbiMsg := context.SbiMsg{
		UeContextId: ueContextID,
		ReqUri:      infoClassQuery,
		Msg:         supportedFeaturesQuery,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	var ueContextInfo *models.UeContextInfo
	ue.EventChannel.UpdateSbiHandler(MtHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result
	if msg.RespData != nil {
		ueContextInfo = msg.RespData.(*models.UeContextInfo)
	}
	//ueContextInfo, problemDetails := ProvideDomainSelectionInfoProcedure(ueContextID,
	//	infoClassQuery, supportedFeaturesQuery)
	if msg.ProblemDetails != nil {
		return http_wrapper.NewResponse(int(msg.ProblemDetails.(models.ProblemDetails).Status), nil, msg.ProblemDetails.(models.ProblemDetails))
	} else {
		return http_wrapper.NewResponse(http.StatusOK, nil, ueContextInfo)
	}
}

func ProvideDomainSelectionInfoProcedure(ueContextID string, infoClassQuery string, supportedFeaturesQuery string) (
	*models.UeContextInfo, *models.ProblemDetails) {
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := &models.ProblemDetails{
			Status: http.StatusNotFound,
			Cause:  "CONTEXT_NOT_FOUND",
		}
		return nil, problemDetails
	}

	ueContextInfo := new(models.UeContextInfo)

	// TODO: Error Status 307, 403 in TS29.518 Table 6.3.3.3.3.1-3
	anType := ue.GetAnType()
	if anType != "" && infoClassQuery != "" {
		ranUe := ue.RanUe[anType]
		ueContextInfo.AccessType = anType
		ueContextInfo.LastActTime = ranUe.LastActTime
		ueContextInfo.RatType = ue.RatType
		ueContextInfo.SupportedFeatures = ranUe.SupportedFeatures
		ueContextInfo.SupportVoPS = ranUe.SupportVoPS
		ueContextInfo.SupportVoPSn3gpp = ranUe.SupportVoPSn3gpp
	}

	return ueContextInfo, nil
}
