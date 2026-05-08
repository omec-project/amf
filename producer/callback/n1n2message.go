// SPDX-FileCopyrightText: 2024 Intel Corporation
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package callback

import (
	"context"
	"io"
	"os"
	"strconv"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Namf_Communication"
	"github.com/omec-project/openapi/v2/models"
)

func SendN1N2TransferFailureNotification(ue *amf_context.AmfUe, cause models.N1N2MessageTransferCause) {
	if ue.N1N2Message == nil {
		logger.CallbackLog.Warnln("N1N2 Message Transfer Failure Notification not sent")
		return
	}
	n1n2Message := ue.N1N2Message
	uri := n1n2Message.Request.JsonData.GetN1n2FailureTxfNotifURI()
	if n1n2Message.Status != models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE || uri == "" {
		logger.CallbackLog.Warnln("N1N2 Message Transfer Failure Notification not sent")
		return
	}
	cfg := Namf_Communication.NewConfiguration()
	serverConfig := &cfg.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = n1n2Message.Request.JsonData.GetN1n2FailureTxfNotifURI()
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Namf_Communication.NewAPIClient(cfg)

	n1N2MsgTxfrFailureNotification := models.N1N2MsgTxfrFailureNotification{
		Cause:          cause,
		N1n2MsgDataUri: n1n2Message.ResourceUri,
	}

	apiN1N2TransferFailureNotificationRequest := client.N1N2MessageCollectionCollectionCallbackN1N2TransferFailureAPI.
		N1N2TransferFailureNotification(context.Background())
	apiN1N2TransferFailureNotificationRequest = apiN1N2TransferFailureNotificationRequest.N1N2MsgTxfrFailureNotification(n1N2MsgTxfrFailureNotification)
	httpResponse, err := client.N1N2MessageCollectionCollectionCallbackN1N2TransferFailureAPI.
		N1N2TransferFailureNotificationExecute(apiN1N2TransferFailureNotificationRequest)

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

func SendN1MessageNotify(ue *amf_context.AmfUe, n1class models.N1MessageClass, n1Msg []byte,
	registerContext *models.RegistrationContextContainer,
) {
	ue.N1N2MessageSubscription.Range(func(key, value interface{}) bool {
		subscriptionID := key.(int64)
		subscription := value.(models.UeN1N2InfoSubscriptionCreateData)

		if subscription.GetN1NotifyCallbackUri() != "" && subscription.GetN1MessageClass() == n1class {
			cfg := Namf_Communication.NewConfiguration()
			serverConfig := &cfg.Servers[0]
			if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
				apiRootVar.DefaultValue = subscription.GetN1NotifyCallbackUri()
				serverConfig.Variables["apiRoot"] = apiRootVar
			}
			client := Namf_Communication.NewAPIClient(cfg)

			// Create a temporary file
			tmpFile, err := os.CreateTemp("", "prefix")
			if err != nil {
				logger.ProducerLog.Errorln(err)
			}
			defer tmpFile.Close()
			if _, err = tmpFile.Write(n1Msg); err != nil {
				logger.ProducerLog.Errorln(err)
			}
			if _, err = tmpFile.Seek(0, io.SeekStart); err != nil {
				logger.ProducerLog.Errorln(err)
			}

			jsonData := models.N1MessageNotification{
				N1NotifySubscriptionId: openapi.PtrString(strconv.Itoa(int(subscriptionID))),
				N1MessageContainer: models.N1MessageContainer{
					N1MessageClass: subscription.GetN1MessageClass(),
					N1MessageContent: models.RefToBinaryData{
						ContentId: "n1Msg",
					},
				},
				RegistrationCtxtContainer: registerContext,
			}

			apiN1MessageNotifyRequest := client.N1N2SubscriptionsCollectionForIndividualUEContextsCollectionCallbackN1N2MessageNotifyn1NotifyCallbackUriAPI.
				N1MessageNotify(context.Background())
			apiN1MessageNotifyRequest = apiN1MessageNotifyRequest.JsonData(jsonData)
			apiN1MessageNotifyRequest = apiN1MessageNotifyRequest.BinaryDataN1Message(tmpFile)
			httpResponse, err := client.N1N2SubscriptionsCollectionForIndividualUEContextsCollectionCallbackN1N2MessageNotifyn1NotifyCallbackUriAPI.
				N1MessageNotifyExecute(apiN1MessageNotifyRequest)
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
	var callbackUri string
	for _, subscription := range ue.TargetAmfProfile.DefaultNotificationSubscriptions {
		if subscription.GetNotificationType() == models.NOTIFICATIONTYPE_N1_MESSAGES &&
			subscription.GetN1MessageClass() == models.N1MESSAGECLASS__5_GMM {
			callbackUri = subscription.GetCallbackUri()
			break
		}
	}

	cfg := Namf_Communication.NewConfiguration()
	serverConfig := &cfg.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = callbackUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Namf_Communication.NewAPIClient(cfg)

	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "prefix")
	if err != nil {
		logger.ProducerLog.Errorln(err)
	}
	defer tmpFile.Close()
	if _, err = tmpFile.Write(n1Msg); err != nil {
		logger.ProducerLog.Errorln(err)
	}
	if _, err = tmpFile.Seek(0, io.SeekStart); err != nil {
		logger.ProducerLog.Errorln(err)
	}

	jsonData := models.N1MessageNotification{
		N1MessageContainer: models.N1MessageContainer{
			N1MessageClass: models.N1MESSAGECLASS__5_GMM,
			N1MessageContent: models.RefToBinaryData{
				ContentId: "n1Msg",
			},
		},
		RegistrationCtxtContainer: registerContext,
	}

	apiN1MessageNotifyRequest := client.N1N2SubscriptionsCollectionForIndividualUEContextsCollectionCallbackN1N2MessageNotifyn1NotifyCallbackUriAPI.
		N1MessageNotify(context.Background())
	apiN1MessageNotifyRequest = apiN1MessageNotifyRequest.JsonData(jsonData)
	apiN1MessageNotifyRequest = apiN1MessageNotifyRequest.BinaryDataN1Message(tmpFile)
	httpResp, err := client.N1N2SubscriptionsCollectionForIndividualUEContextsCollectionCallbackN1N2MessageNotifyn1NotifyCallbackUriAPI.
		N1MessageNotifyExecute(apiN1MessageNotifyRequest)
	if err != nil {
		if httpResp == nil {
			logger.HttpLog.Errorln(err.Error())
		} else if err.Error() != httpResp.Status {
			logger.HttpLog.Errorln(err.Error())
		}
	}
}
