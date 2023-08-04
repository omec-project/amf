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
	"github.com/omec-project/openapi/models"

	"github.com/omec-project/amf/producer"
	"github.com/omec-project/http_wrapper"
	"github.com/omec-project/openapi"
)

func HTTPNfSubscriptionStatusNotify(c *gin.Context) {
	var nfSubscriptionStatusNotification models.NotificationData

	requestBody, err := c.GetRawData()
	if err != nil {
		logger.CallbackLog.Errorf("Get Request Body error: %+v", err)
		problemDetail := models.ProblemDetails{
			Title:  "System failure",
			Status: http.StatusInternalServerError,
			Detail: err.Error(),
			Cause:  "SYSTEM_FAILURE",
		}
		c.JSON(http.StatusInternalServerError, problemDetail)
		return
	}

	err = openapi.Deserialize(&nfSubscriptionStatusNotification, requestBody, "application/json")
	if err != nil {
		problemDetail := "[Request Body] " + err.Error()
		rsp := models.ProblemDetails{
			Title:  "Malformed request syntax",
			Status: http.StatusBadRequest,
			Detail: problemDetail,
		}
		logger.CallbackLog.Errorln(problemDetail)
		c.JSON(http.StatusBadRequest, rsp)
		return
	}

	req := http_wrapper.NewRequest(c.Request, nfSubscriptionStatusNotification)

	rsp := producer.HandleNfSubscriptionStatusNotify(req)

	responseBody, err := openapi.Serialize(rsp.Body, "application/json")
	if err != nil {
		logger.CallbackLog.Errorln(err)
		problemDetails := models.ProblemDetails{
			Status: http.StatusInternalServerError,
			Cause:  "SYSTEM_FAILURE",
			Detail: err.Error(),
		}
		c.JSON(http.StatusInternalServerError, problemDetails)
	} else if rsp.Body != nil {
		c.Data(rsp.Status, "application/json", responseBody)
	}
}
