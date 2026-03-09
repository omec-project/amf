// SPDX-FileCopyrightText: 2024 Intel Corporation
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package callback

import (
	"context"
	"strconv"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/Namf_Communication"
	"github.com/omec-project/openapi/models"
)

func SendN1N2TransferFailureNotification(ue *amf_context.AmfUe, cause models.N1N2MessageTransferCause) {
	if ue.N1N2Message == nil {
		return
	}
	n1n2Message := ue.N1N2Message
	uri := n1n2Message.Request.JsonData.N1n2FailureTxfNotifURI
	if n1n2Message.Status == models.N1N2MessageTransferCause_ATTEMPTING_TO_REACH_UE && uri != "" {
		configuration := Namf_Communication.NewConfiguration()
		client := Namf_Communication.NewAPIClient(configuration)

		n1N2MsgTxfrFailureNotification := models.N1N2MsgTxfrFailureNotification{
			Cause:          cause,
			N1n2MsgDataUri: n1n2Message.ResourceUri,
		}

		httpResponse, err := client.N1N2MessageTransferStatusNotificationCallbackDocumentApi.
			N1N2TransferFailureNotification(context.Background(), uri, n1N2MsgTxfrFailureNotification)

		if err != nil {
			if httpResponse == nil {
				logger.HttpLog.Errorln(err.Error())
			} else if err.Error() != httpResponse.Status {
				logger.HttpLog.Errorln(err.Error())
			}
		} else {
			ue.N1N2Message = nil
		}
	}
}

func SendN1MessageNotify(ue *amf_context.AmfUe, n1class models.N1MessageClass, n1Msg []byte,
	registerContext *models.RegistrationContextContainer,
) {
	ue.N1N2MessageSubscription.Range(func(key, value interface{}) bool {
		subscriptionID := key.(int64)
		subscription := value.(models.UeN1N2InfoSubscriptionCreateData)

		if subscription.N1NotifyCallbackUri != "" && subscription.N1MessageClass == n1class {
			configuration := Namf_Communication.NewConfiguration()
			client := Namf_Communication.NewAPIClient(configuration)
			n1MessageNotify := models.N1MessageNotify{
				JsonData: &models.N1MessageNotification{
					N1NotifySubscriptionId: strconv.Itoa(int(subscriptionID)),
					N1MessageContainer: &models.N1MessageContainer{
						N1MessageClass: subscription.N1MessageClass,
						N1MessageContent: &models.RefToBinaryData{
							ContentId: "n1Msg",
						},
					},
					RegistrationCtxtContainer: registerContext,
				},
				BinaryDataN1Message: n1Msg,
			}
			httpResponse, err := client.N1MessageNotifyCallbackDocumentApiServiceCallbackDocumentApi.
				N1MessageNotify(context.Background(), subscription.N1NotifyCallbackUri, n1MessageNotify)
			if err != nil {
				if httpResponse == nil {
					logger.HttpLog.Errorln(err.Error())
				} else if err.Error() != httpResponse.Status {
					logger.HttpLog.Errorln(err.Error())
				}
			}
		}
		return true
	})
}

// TS 29.518 5.2.2.3.5.2
func SendN1MessageNotifyAtAMFReAllocation(
	ue *amf_context.AmfUe, n1Msg []byte, registerContext *models.RegistrationContextContainer,
) {
	configuration := Namf_Communication.NewConfiguration()
	client := Namf_Communication.NewAPIClient(configuration)

	n1MessageNotify := models.N1MessageNotify{
		JsonData: &models.N1MessageNotification{
			N1MessageContainer: &models.N1MessageContainer{
				N1MessageClass: models.N1MessageClass__5_GMM,
				N1MessageContent: &models.RefToBinaryData{
					ContentId: "n1Msg",
				},
			},
			RegistrationCtxtContainer: registerContext,
		},
		BinaryDataN1Message: n1Msg,
	}

	var callbackUri string
	for _, subscription := range ue.TargetAmfProfile.DefaultNotificationSubscriptions {
		if subscription.NotificationType == models.NotificationType_N1_MESSAGES &&
			subscription.N1MessageClass == models.N1MessageClass__5_GMM {
			callbackUri = subscription.CallbackUri
			break
		}
	}

	httpResp, err := client.N1MessageNotifyCallbackDocumentApiServiceCallbackDocumentApi.
		N1MessageNotify(context.Background(), callbackUri, n1MessageNotify)
	if err != nil {
		if httpResp == nil {
			logger.HttpLog.Errorln(err.Error())
		} else if err.Error() != httpResp.Status {
			logger.HttpLog.Errorln(err.Error())
		}
	}
}
