// SPDX-FileCopyrightText: 2024 Intel Corporation
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package callback

import (
	"context"
	"reflect"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2/models"
)

func SendAmfStatusChangeNotify(amfStatus models.StatusChange, guamiList []models.Guami) {
	amfSelf := amf_context.AMF_Self()

	amfSelf.AMFStatusSubscriptions.Range(func(key, value interface{}) bool {
		subscriptionData := value.(models.SubscriptionDataAmf)
		amfStatusNotification := models.AmfStatusChangeNotification{}
		amfStatusInfo := models.AmfStatusInfo{}

		for _, guami := range guamiList {
			for _, subGumi := range subscriptionData.GuamiList {
				if reflect.DeepEqual(guami, subGumi) {
					// AMF status is available
					amfStatusInfo.GuamiList = append(amfStatusInfo.GuamiList, guami)
				}
			}
		}

		amfStatusInfo.StatusChange = amfStatus

		amfStatusNotification.AmfStatusInfoList = append(amfStatusNotification.AmfStatusInfoList, amfStatusInfo)

		logger.ProducerLog.Infof("[AMF] Send Amf Status Change Notify to %s", subscriptionData.AmfStatusUri)
		httpResponse, err := postCallbackJSON(context.Background(), subscriptionData.AmfStatusUri, amfStatusNotification)
		defer closeCallbackResponseBody(httpResponse)
		logCallbackResponseError(httpResponse, err)
		return true
	})
}
