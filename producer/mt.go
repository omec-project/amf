// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package producer

import (
	ctxt "context"
	"net/http"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/httpwrapper"
)

func MtHandler(ctx ctxt.Context, s1, s2 string, msg interface{}) (interface{}, string, interface{}, interface{}) {
	switch msg := msg.(type) {
	case string:
		r1, r2 := ProvideDomainSelectionInfoProcedure(s1, s2, msg)
		return r1, "", r2, nil
	}

	return nil, "", nil, nil
}

func HandleProvideDomainSelectionInfoRequest(request *httpwrapper.Request) *httpwrapper.Response {
	var ue *context.AmfUe
	var ok bool
	logger.MtLog.Info("Handle Provide Domain Selection Info Request")

	ueContextID := request.Params["ueContextId"]
	infoClassQuery := request.Query.Get("info-class")
	supportedFeaturesQuery := request.Query.Get("supported-features")

	amfSelf := context.AMF_Self()

	if ue, ok = amfSelf.AmfUeFindByUeContextID(ueContextID); !ok {
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusNotFound)
		problemDetails.SetCause("CONTEXT_NOT_FOUND")
		return httpwrapper.NewResponse(http.StatusNotFound, nil, problemDetails)
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
	// ueContextInfo, problemDetails := ProvideDomainSelectionInfoProcedure(ueContextID,
	//	infoClassQuery, supportedFeaturesQuery)
	if msg.ProblemDetails != nil {
		problemDetails := msg.ProblemDetails.(*models.ProblemDetails)
		status := problemDetails.GetStatus()
		if status == 0 {
			status = http.StatusInternalServerError
		}
		return httpwrapper.NewResponse(int(status), nil, problemDetails)
	} else {
		return httpwrapper.NewResponse(http.StatusOK, nil, ueContextInfo)
	}
}

func ProvideDomainSelectionInfoProcedure(ueContextID string, infoClassQuery string, supportedFeaturesQuery string) (
	*models.UeContextInfo, *models.ProblemDetails,
) {
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusNotFound)
		problemDetails.SetCause("CONTEXT_NOT_FOUND")
		return nil, problemDetails
	}

	ueContextInfo := models.NewUeContextInfo()

	// TODO: Error Status 307, 403 in TS29.518 Table 6.3.3.3.3.1-3
	anType := ue.GetAnType()
	if anType != "" && infoClassQuery != "" {
		ranUe := ue.RanUe[anType]
		ueContextInfo.SetAccessType(anType)
		if ranUe != nil {
			if ranUe.LastActTime != nil {
				ueContextInfo.SetLastActTime(*ranUe.LastActTime)
			}
			ueContextInfo.SetSupportedFeatures(ranUe.SupportedFeatures)
			ueContextInfo.SetSupportVoPS(ranUe.SupportVoPS)
			ueContextInfo.SetSupportVoPSn3gpp(ranUe.SupportVoPSn3gpp)
		}
		ueContextInfo.SetRatType(ue.RatType)
	}

	return ueContextInfo, nil
}
