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
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
)

func LocationInfoHandler(ctx ctxt.Context, s1, s2 string, msg interface{}) (interface{}, string, interface{}, interface{}) {
	switch msg := msg.(type) {
	case models.RequestLocInfo:
		r1, r2 := ProvideLocationInfoProcedure(msg, s1)
		return r1, "", r2, nil
	}

	return nil, "", nil, nil
}

func HandleProvideLocationInfoRequest(request *httpwrapper.Request) *httpwrapper.Response {
	var ue *context.AmfUe
	var ok bool
	logger.ProducerLog.Info("Handle Provide Location Info Request")

	requestLocInfo := request.Body.(models.RequestLocInfo)
	ueContextID := request.Params["ueContextId"]

	amfSelf := context.AMF_Self()
	if ue, ok = amfSelf.AmfUeFindByUeContextID(ueContextID); !ok {
		return httpwrapper.NewResponse(http.StatusNotFound, nil, utils.ProblemDetailsContextNotFound(""))
	}

	sbiMsg := context.SbiMsg{
		UeContextId: ueContextID,
		ReqUri:      "",
		Msg:         requestLocInfo,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	var provideLocInfo *models.ProvideLocInfo
	ue.EventChannel.UpdateSbiHandler(LocationInfoHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result
	if msg.RespData != nil {
		provideLocInfo = msg.RespData.(*models.ProvideLocInfo)
	}
	// provideLocInfo, problemDetails := ProvideLocationInfoProcedure(requestLocInfo, ueContextID)
	if msg.ProblemDetails != nil {
		problemDetails := msg.ProblemDetails.(*models.ProblemDetails)
		status := problemDetails.GetStatus()
		if status == 0 {
			status = http.StatusInternalServerError
		}
		return httpwrapper.NewResponse(int(status), nil, problemDetails)
	} else {
		return httpwrapper.NewResponse(http.StatusOK, nil, provideLocInfo)
	}
}

func ProvideLocationInfoProcedure(requestLocInfo models.RequestLocInfo, ueContextID string) (
	*models.ProvideLocInfo, *models.ProblemDetails,
) {
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByUeContextID(ueContextID)
	if !ok {
		return nil, utils.ProblemDetailsContextNotFound("")
	}

	anType := ue.GetAnType()
	if anType == "" {
		return nil, utils.ProblemDetailsContextNotFound("")
	}

	provideLocInfo := models.NewProvideLocInfo()

	ranUe := ue.RanUe[anType]
	if requestLocInfo.GetReq5gsLoc() || requestLocInfo.GetReqCurrentLoc() {
		provideLocInfo.CurrentLoc = openapi.PtrBool(true)
		provideLocInfo.Location = &ue.Location
	}

	if requestLocInfo.GetReqRatType() {
		provideLocInfo.RatType = &ue.RatType
	}

	if requestLocInfo.GetReqTimeZone() {
		provideLocInfo.Timezone = openapi.PtrString(ue.TimeZone)
	}

	if ranUe != nil && requestLocInfo.GetSupportedFeatures() != "" {
		provideLocInfo.SupportedFeatures = openapi.PtrString(ranUe.SupportedFeatures)
	}
	return provideLocInfo, nil
}
