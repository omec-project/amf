// SPDX-FileCopyrightText: 2022 Infosys Limited
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package httpcallback

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/producer"
	"github.com/omec-project/openapi"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/openapi/utils"
	"github.com/omec-project/util/httpwrapper"
)

func HTTPDeregistrationNotification(c *gin.Context) {
	var deregistrationData models.DeregistrationData

	requestBody, err := c.GetRawData()
	if err != nil {
		logger.CallbackLog.Errorf("Get Request Body error: %+v", err)
		problemDetail := utils.ProblemDetailsSystemFailure(err.Error())
		c.JSON(http.StatusInternalServerError, problemDetail)
		return
	}

	err = openapi.Decode(&deregistrationData, requestBody, "application/json")
	if err != nil {
		problemDetail := "[Request Body] " + err.Error()
		rsp := utils.ProblemDetailsMalformedRequestSyntax(problemDetail)
		logger.CallbackLog.Errorln(problemDetail)
		c.JSON(http.StatusBadRequest, rsp)
		return
	}

	req := httpwrapper.NewRequest(c.Request, deregistrationData)
	if supi, exists := c.Params.Get("supi"); exists {
		req.Params["supi"] = supi
	}
	rsp := producer.HandleDeregistrationNotification(c.Request.Context(), req)

	responseBody, err := openapi.SetBody(rsp.Body, "application/json")
	if err != nil {
		logger.CallbackLog.Errorln(err)
		problemDetails := utils.ProblemDetailsSystemFailure(err.Error())
		c.JSON(http.StatusInternalServerError, problemDetails)
	} else {
		c.Data(rsp.Status, "application/json", responseBody.Bytes())
	}
}
