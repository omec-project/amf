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
	"github.com/sirupsen/logrus"
)

var HttpLog *logrus.Entry

func init() {
	HttpLog = logger.HttpLog
}

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
				HttpLog.Errorln(err.Error())
			} else if err.Error() != httpResponse.Status {
				HttpLog.Errorln(err.Error())
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
					HttpLog.Errorln(err.Error())
				} else if err.Error() != httpResponse.Status {
					HttpLog.Errorln(err.Error())
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
			HttpLog.Errorln(err.Error())
		} else if err.Error() != httpResp.Status {
			HttpLog.Errorln(err.Error())
		}
	}
}

func SendN2InfoNotify(ue *amf_context.AmfUe, n2class models.N2InformationClass, n1Msg, n2Msg []byte) {
	ue.N1N2MessageSubscription.Range(func(key, value interface{}) bool {
		subscriptionID := key.(int64)
		subscription := value.(models.UeN1N2InfoSubscriptionCreateData)

		if subscription.N2NotifyCallbackUri != "" && subscription.N2InformationClass == n2class {
			configuration := Namf_Communication.NewConfiguration()
			client := Namf_Communication.NewAPIClient(configuration)

			n2InformationNotify := models.N2InfoNotifyRequest{
				JsonData: &models.N2InformationNotification{
					N2NotifySubscriptionId: strconv.Itoa(int(subscriptionID)),
					N2InfoContainer: &models.N2InfoContainer{
						N2InformationClass: n2class,
					},
				},
				BinaryDataN1Message:     n1Msg,
				BinaryDataN2Information: n2Msg,
			}
			if n2Msg == nil {
				HttpLog.Errorln("Send N2 Info Notify Error(N2 Info does not exist)")
			}
			switch n2class {
			case models.N2InformationClass_SM:
				n2InformationNotify.JsonData.N2InfoContainer.SmInfo = &models.N2SmInformation{
					N2InfoContent: &models.N2InfoContent{
						NgapData: &models.RefToBinaryData{
							ContentId: "n2Info",
						},
					},
				}
			case models.N2InformationClass_NRP_PA:
				n2InformationNotify.JsonData.N2InfoContainer.NrppaInfo = &models.NrppaInformation{
					NrppaPdu: &models.N2InfoContent{
						NgapData: &models.RefToBinaryData{
							ContentId: "n2Info",
						},
					},
				}
			case models.N2InformationClass_PWS, models.N2InformationClass_PWS_BCAL, models.N2InformationClass_PWS_RF:
				n2InformationNotify.JsonData.N2InfoContainer.PwsInfo = &models.PwsInformation{
					PwsContainer: &models.N2InfoContent{
						NgapData: &models.RefToBinaryData{
							ContentId: "n2Info",
						},
					},
				}
			case models.N2InformationClass_RAN:
				n2InformationNotify.JsonData.N2InfoContainer.RanInfo = &models.N2RanInformation{
					N2InfoContent: &models.N2InfoContent{
						NgapData: &models.RefToBinaryData{
							ContentId: "n2Info",
						},
					},
				}
			}

			httpResponse, err := client.N2InfoNotifyCallbackDocumentApiServiceCallbackDocumentApi.
				N2InfoNotify(context.Background(), subscription.N2NotifyCallbackUri, n2InformationNotify)
			if err != nil {
				if httpResponse == nil {
					HttpLog.Errorln(err.Error())
				} else if err.Error() != httpResponse.Status {
					HttpLog.Errorln(err.Error())
				}
			}
		}
		return true
	})
}
