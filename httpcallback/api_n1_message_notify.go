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
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
)

func HTTPN1MessageNotify(c *gin.Context) {
	var n1MessageNotification models.N1MessageNotification

	requestBody, err := c.GetRawData()
	if err != nil {
		logger.CallbackLog.Errorf("Get Request Body error: %+v", err)
		problemDetail := utils.ProblemDetailsSystemFailure(err.Error())
		c.JSON(http.StatusInternalServerError, problemDetail)
		return
	}

	err = openapi.Decode(&n1MessageNotification, requestBody, "application/json")
	if err != nil {
		problemDetail := "[Request Body] " + err.Error()
		rsp := utils.ProblemDetailsMalformedRequestSyntax(problemDetail)
		logger.CallbackLog.Errorln(problemDetail)
		c.JSON(http.StatusBadRequest, rsp)
		return
	}

	req := httpwrapper.NewRequest(c.Request, n1MessageNotification)

	rsp := producer.HandleN1MessageNotify(req)

	responseBody, err := openapi.SetBody(rsp.Body, "application/json")
	if err != nil {
		logger.CallbackLog.Errorln(err)
		problemDetails := utils.ProblemDetailsSystemFailure(err.Error())
		c.JSON(http.StatusInternalServerError, problemDetails)
	} else {
		c.Data(rsp.Status, "application/json", responseBody.Bytes())
	}
}
