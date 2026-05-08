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
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Namf_Communication"
	"github.com/omec-project/openapi/v2/models"
)

func SendAmfStatusChangeNotify(amfStatus models.StatusChange, guamiList []models.Guami) {
	amfSelf := amf_context.AMF_Self()

	amfSelf.AMFStatusSubscriptions.Range(func(key, value interface{}) bool {
		subscriptionData := value.(models.SubscriptionDataAmf)

		cfg := Namf_Communication.NewConfiguration()
		serverConfig := &cfg.Servers[0]
		if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
			apiRootVar.DefaultValue = subscriptionData.AmfStatusUri
			serverConfig.Variables["apiRoot"] = apiRootVar
		}
		client := Namf_Communication.NewAPIClient(cfg)
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

		amfStatusInfo = models.AmfStatusInfo{
			StatusChange:     amfStatus,
			TargetAmfRemoval: openapi.PtrString(""),
			TargetAmfFailure: openapi.PtrString(""),
		}

		amfStatusNotification.AmfStatusInfoList = append(amfStatusNotification.AmfStatusInfoList, amfStatusInfo)

		logger.ProducerLog.Infof("[AMF] Send Amf Status Change Notify to %s", subscriptionData.AmfStatusUri)
		apiAmfStatusChangeNotifyRequest := client.SubscriptionsCollectionCollectionCallbackAmfStatusChangeAPI.AmfStatusChangeNotifyOnSubscriptionUpdate(context.Background())
		apiAmfStatusChangeNotifyRequest = apiAmfStatusChangeNotifyRequest.AmfStatusChangeNotification(amfStatusNotification)
		httpResponse, err := client.SubscriptionsCollectionCollectionCallbackAmfStatusChangeAPI.
			AmfStatusChangeNotifyOnSubscriptionUpdateExecute(apiAmfStatusChangeNotifyRequest)
		if err != nil {
			if httpResponse == nil {
				logger.HttpLog.Errorln(err.Error())
			} else if err.Error() != httpResponse.Status {
				logger.HttpLog.Errorln(err.Error())
			}
		}
		return true
	})
}
