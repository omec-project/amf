// SPDX-FileCopyrightText: 2024 Intel Corporation
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package callback

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func createTempBinaryFile(data []byte) (*os.File, error) {
	tmpFile, err := os.CreateTemp("", "prefix")
	if err != nil {
		return nil, err
	}
	if _, err = tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, err
	}
	if _, err = tmpFile.Seek(0, io.SeekStart); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return nil, err
	}
	return tmpFile, nil
}

func cleanupTempBinaryFile(tmpFile *os.File) {
	if tmpFile == nil {
		return
	}
	if err := tmpFile.Close(); err != nil {
		logger.ProducerLog.Errorln(err)
	}
	if err := os.Remove(tmpFile.Name()); err != nil {
		logger.ProducerLog.Errorln(err)
	}
}

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

	n1N2MsgTxfrFailureNotification := models.N1N2MsgTxfrFailureNotification{
		Cause:          cause,
		N1n2MsgDataUri: n1n2Message.ResourceUri,
	}
	httpResponse, err := postCallbackJSON(context.Background(), uri, n1N2MsgTxfrFailureNotification)
	defer closeCallbackResponseBody(httpResponse)
	if err == nil && httpResponse != nil && httpResponse.StatusCode < 300 {
		ue.N1N2Message = nil
		return
	}
	logCallbackResponseError(httpResponse, err)
}

func SendN1MessageNotify(ue *amf_context.AmfUe, n1class models.N1MessageClass, n1Msg []byte,
	registerContext *models.RegistrationContextContainer,
) {
	ue.N1N2MessageSubscription.Range(func(key, value interface{}) bool {
		subscriptionID := key.(int64)
		subscription := value.(models.UeN1N2InfoSubscriptionCreateData)

		if subscription.GetN1NotifyCallbackUri() != "" && subscription.GetN1MessageClass() == n1class {
			tmpFile, err := createTempBinaryFile(n1Msg)
			if err != nil {
				logger.ProducerLog.Errorln(fmt.Errorf("create N1 message temp file: %w", err))
				return true
			}
			defer cleanupTempBinaryFile(tmpFile)

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

			n1MessageNotifyRequest := models.NewN1MessageNotifyRequest()
			n1MessageNotifyRequest.SetJsonData(jsonData)
			n1MessageNotifyRequest.SetBinaryDataN1Message(tmpFile)
			httpResponse, err := postCallbackMultipart(context.Background(), subscription.GetN1NotifyCallbackUri(), n1MessageNotifyRequest)
			defer closeCallbackResponseBody(httpResponse)
			logCallbackResponseError(httpResponse, err)
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

	tmpFile, err := createTempBinaryFile(n1Msg)
	if err != nil {
		logger.ProducerLog.Errorln(fmt.Errorf("create AMF re-allocation N1 message temp file: %w", err))
		return
	}
	defer cleanupTempBinaryFile(tmpFile)

	jsonData := models.N1MessageNotification{
		N1MessageContainer: models.N1MessageContainer{
			N1MessageClass: models.N1MESSAGECLASS__5_GMM,
			N1MessageContent: models.RefToBinaryData{
				ContentId: "n1Msg",
			},
		},
		RegistrationCtxtContainer: registerContext,
	}

	n1MessageNotifyRequest := models.NewN1MessageNotifyRequest()
	n1MessageNotifyRequest.SetJsonData(jsonData)
	n1MessageNotifyRequest.SetBinaryDataN1Message(tmpFile)
	httpResponse, err := postCallbackMultipart(context.Background(), callbackUri, n1MessageNotifyRequest)
	defer closeCallbackResponseBody(httpResponse)
	logCallbackResponseError(httpResponse, err)
}
