// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package producer

import (
	ctxt "context"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"

	"github.com/mohae/deepcopy"
	"github.com/omec-project/amf/consumer"
	"github.com/omec-project/amf/context"
	gmm_message "github.com/omec-project/amf/gmm/message"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/nas"
	ngap_message "github.com/omec-project/amf/ngap/message"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/nas/v2/nasConvert"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	nrfCache "github.com/omec-project/openapi/v2/nrfcache"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
)

func SmContextHandler(ctx ctxt.Context, s1, s2 string, msg interface{}) (interface{}, string, interface{}, interface{}) {
	switch msg := msg.(type) {
	case models.SmContextStatusNotification:
		var pduSessionID int
		if pduSessionIDTmp, err := strconv.Atoi(s2); err != nil {
			logger.ProducerLog.Warnf("PDU Session ID atoi failed: %+v", err)
		} else {
			pduSessionID = pduSessionIDTmp
		}
		r1 := SmContextStatusNotifyProcedure(ctx, s1, int32(pduSessionID), msg)
		return nil, "", r1, nil
	case models.PolicyUpdate:
		r1 := AmPolicyControlUpdateNotifyUpdateProcedure(s1, msg)
		return nil, "", r1, nil
	case models.TerminationNotification:
		r1 := AmPolicyControlUpdateNotifyTerminateProcedure(ctx, s1, msg)
		return nil, "", r1, nil
	}

	return nil, "", nil, nil
}

func HandleSmContextStatusNotify(request *httpwrapper.Request) *httpwrapper.Response {
	var ue *context.AmfUe
	var ok bool
	logger.ProducerLog.Infoln("[AMF] handle SmContext Status Notify")

	guti := request.Params["guti"]
	pduSessionIDString := request.Params["pduSessionId"]

	amfSelf := context.AMF_Self()
	ue, ok = amfSelf.AmfUeFindByGuti(guti)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound(fmt.Sprintf("Guti[%s] Not Found", guti))
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}

	smContextStatusNotification := request.Body.(models.SmContextStatusNotification)
	sbiMsg := context.SbiMsg{
		UeContextId: guti,
		ReqUri:      pduSessionIDString,
		Msg:         smContextStatusNotification,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	ue.EventChannel.UpdateSbiHandler(SmContextHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result
	// problemDetails := SmContextStatusNotifyProcedure(guti, int32(pduSessionID), smContextStatusNotification)
	if msg.ProblemDetails != nil {
		return httpwrapper.NewResponse(int(msg.ProblemDetails.(*models.ProblemDetails).GetStatus()), nil, msg.ProblemDetails.(*models.ProblemDetails))
	} else {
		return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}

func SmContextStatusNotifyProcedure(ctx ctxt.Context, guti string, pduSessionID int32,
	smContextStatusNotification models.SmContextStatusNotification,
) *models.ProblemDetails {
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByGuti(guti)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound(fmt.Sprintf("Guti[%s] Not Found", guti))
		return problemDetails
	}

	smContext, ok := ue.SmContextFindByPDUSessionID(pduSessionID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound(fmt.Sprintf("PDUSessionID[%d] Not Found", pduSessionID))
		return problemDetails
	}

	if smContextStatusNotification.StatusInfo.ResourceStatus == models.RESOURCESTATUS_RELEASED {
		ue.ProducerLog.Debugf("release PDU Session[%d] (Cause: %s)", pduSessionID,
			smContextStatusNotification.StatusInfo.Cause)

		if smContext.PduSessionIDDuplicated() {
			ue.ProducerLog.Debugf("resume establishing PDU Session[%d]", pduSessionID)
			smContext.SetDuplicatedPduSessionID(false)
			go func() {
				var (
					snssai    models.Snssai
					dnn       string
					smMessage []byte
				)
				smMessage = smContext.ULNASTransport().GetPayloadContainerContents()
				anType := smContext.AccessType()

				if smContext.ULNASTransport().SNSSAI != nil {
					snssai = nasConvert.SnssaiToModels(smContext.ULNASTransport().SNSSAI)
				} else {
					if allowedNssai, ok := ue.AllowedNssai[anType]; ok {
						snssai = allowedNssai[0].AllowedSnssai
					} else {
						ue.GmmLog.Errorln("UE doesn't have allowedNssai")
						return
					}
				}

				if smContext.ULNASTransport().DNN != nil {
					dnn = string(smContext.ULNASTransport().GetDNN())
				} else {
					if ue.SmfSelectionData != nil {
						snssaiStr := util.SnssaiModelsToHex(snssai)
						if snssaiInfo, ok := ue.SmfSelectionData.GetSubscribedSnssaiInfos()[snssaiStr]; ok {
							for _, dnnInfo := range snssaiInfo.DnnInfos {
								if dnnInfo.GetDefaultDnnIndicator() {
									dnn = *dnnInfo.GetDnn().String
								}
							}
						} else {
							// user's subscription context obtained from UDM does not contain the default DNN for the,
							// S-NSSAI, the AMF shall use a locally configured DNN as the DNN
							dnn = "internet"
						}
					}
				}

				newSmContext, cause, err := consumer.SelectSmf(ctx, ue, anType, pduSessionID, snssai, dnn)
				if err != nil {
					logger.CallbackLog.Error(err)
					gmm_message.SendDLNASTransport(ue.RanUe[anType], anType,
						nasMessage.PayloadContainerTypeN1SMInfo,
						smContext.ULNASTransport().GetPayloadContainerContents(), pduSessionID, cause, nil, 0)
					return
				}

				response, smContextRef, errResponse, problemDetail, err := consumer.SendCreateSmContextRequest(
					ctx, ue, newSmContext, nil, smMessage)
				if response != nil {
					newSmContext.SetSmContextRef(smContextRef)
					newSmContext.SetUserLocation(deepcopy.Copy(ue.Location).(models.UserLocation))
					ue.GmmLog.Infof("create smContext[pduSessionID: %d] Success", pduSessionID)
					ue.StoreSmContext(pduSessionID, newSmContext)
					// TODO: handle response(response N2SmInfo to RAN if exists)
				} else if errResponse != nil {
					ue.ProducerLog.Warnf("PDU Session Establishment Request is rejected by SMF[pduSessionId:%d]", pduSessionID)
					binaryDataN1SmMessage, err1 := io.ReadAll(errResponse.GetBinaryDataN1SmMessage())
					if err1 != nil {
						ue.ProducerLog.Errorf("read binaryDataN1SmMessage failed: %+v", err1)
					}
					gmm_message.SendDLNASTransport(ue.RanUe[anType], anType,
						nasMessage.PayloadContainerTypeN1SMInfo, binaryDataN1SmMessage, pduSessionID, 0, nil, 0)
				} else if err != nil {
					ue.ProducerLog.Errorf("failed to create smContext[pduSessionID: %d], Error[%s]", pduSessionID, err.Error())
				} else {
					ue.ProducerLog.Errorf("failed to create smContext[pduSessionID: %d], Error[%v]", pduSessionID, problemDetail)
				}
				smContext.DeleteULNASTransport()
			}()
		} else {
			ue.SmContextList.Delete(pduSessionID)
		}
	} else {
		problemDetails := utils.ProblemDetailsWithInvalidParams(
			"Invalid message format", http.StatusBadRequest, "",
			[]models.InvalidParam{
				{Param: "StatusInfo.ResourceStatus", Reason: openapi.PtrString("invalid value")},
			})
		problemDetails.SetCause(utils.CauseInvalidMsgFormat)
		return problemDetails
	}
	return nil
}

func HandleAmPolicyControlUpdateNotifyUpdate(request *httpwrapper.Request) *httpwrapper.Response {
	var ue *context.AmfUe
	var ok bool
	logger.ProducerLog.Infoln("handle AM Policy Control Update Notify [Policy update notification]")

	polAssoID := request.Params["polAssoId"]
	policyUpdate := request.Body.(models.PolicyUpdate)

	amfSelf := context.AMF_Self()
	ue, ok = amfSelf.AmfUeFindByPolicyAssociationID(polAssoID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound(fmt.Sprintf("Policy Association ID[%s] Not Found", polAssoID))
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	sbiMsg := context.SbiMsg{
		UeContextId: polAssoID,
		ReqUri:      "",
		Msg:         policyUpdate,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	ue.EventChannel.UpdateSbiHandler(SmContextHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result
	// problemDetails := AmPolicyControlUpdateNotifyUpdateProcedure(polAssoID, policyUpdate)

	if msg.ProblemDetails != nil {
		return httpwrapper.NewResponse(int(msg.ProblemDetails.(*models.ProblemDetails).GetStatus()), nil, msg.ProblemDetails.(*models.ProblemDetails))
	} else {
		return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}

func AmPolicyControlUpdateNotifyUpdateProcedure(polAssoID string,
	policyUpdate models.PolicyUpdate,
) *models.ProblemDetails {
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByPolicyAssociationID(polAssoID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound(fmt.Sprintf("Policy Association ID[%s] Not Found", polAssoID))
		return problemDetails
	}

	ue.AmPolicyAssociation.Triggers = policyUpdate.Triggers
	ue.RequestTriggerLocationChange = false

	for _, trigger := range policyUpdate.Triggers {
		if trigger == models.REQUESTTRIGGER_LOC_CH {
			ue.RequestTriggerLocationChange = true
		}
		//if trigger == models.REQUESTTRIGGER_PRA_CH {
		// TODO: Presence Reporting Area handling (TS 23.503 6.1.2.5, TS 23.501 5.6.11)
		//}
	}

	if policyUpdate.ServAreaRes != nil {
		ue.AmPolicyAssociation.ServAreaRes = policyUpdate.ServAreaRes
	}

	if policyUpdate.GetRfsp() != 0 {
		ue.AmPolicyAssociation.Rfsp = policyUpdate.Rfsp
	}

	if ue != nil {
		// use go routine to write response first to ensure the order of the procedure
		go func() {
			// UE is CM-Connected State
			if ue.CmConnect(models.ACCESSTYPE__3_GPP_ACCESS) {
				gmm_message.SendConfigurationUpdateCommand(ue, models.ACCESSTYPE__3_GPP_ACCESS, nil)
				// UE is CM-IDLE => paging
			} else {
				message, err := gmm_message.BuildConfigurationUpdateCommand(ue, models.ACCESSTYPE__3_GPP_ACCESS, nil)
				if err != nil {
					logger.GmmLog.Errorf("Build Configuration Update Command Failed : %s", err.Error())
					return
				}

				ue.ConfigurationUpdateMessage = message
				ue.SetOnGoing(models.ACCESSTYPE__3_GPP_ACCESS, &context.OnGoingProcedureWithPrio{
					Procedure: context.OnGoingProcedurePaging,
				})

				pkg, err := ngap_message.BuildPaging(ue, nil, false)
				if err != nil {
					logger.NgapLog.Errorf("Build Paging failed : %s", err.Error())
					return
				}
				ngap_message.SendPaging(ue, pkg)
			}
		}()
	}
	return nil
}

// TS 29.507 4.2.4.3
func HandleAmPolicyControlUpdateNotifyTerminate(request *httpwrapper.Request) *httpwrapper.Response {
	var ue *context.AmfUe
	logger.ProducerLog.Infoln("handle AM Policy Control Update Notify [Request for termination of the policy association]")

	polAssoID := request.Params["polAssoId"]
	terminationNotification := request.Body.(models.TerminationNotification)

	amfSelf := context.AMF_Self()
	ue, ok := amfSelf.AmfUeFindByPolicyAssociationID(polAssoID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound(fmt.Sprintf("Policy Association ID[%s] Not Found", polAssoID))
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	sbiMsg := context.SbiMsg{
		UeContextId: polAssoID,
		ReqUri:      "",
		Msg:         terminationNotification,
		Result:      make(chan context.SbiResponseMsg, 10),
	}
	ue.EventChannel.UpdateSbiHandler(SmContextHandler)
	ue.EventChannel.SubmitMessage(sbiMsg)
	msg := <-sbiMsg.Result

	// problemDetails := AmPolicyControlUpdateNotifyTerminateProcedure(polAssoID, terminationNotification)
	if msg.ProblemDetails != nil {
		return httpwrapper.NewResponse(int(msg.ProblemDetails.(*models.ProblemDetails).GetStatus()), nil, msg.ProblemDetails.(*models.ProblemDetails))
	} else {
		return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}

func AmPolicyControlUpdateNotifyTerminateProcedure(ctx ctxt.Context, polAssoID string,
	terminationNotification models.TerminationNotification,
) *models.ProblemDetails {
	amfSelf := context.AMF_Self()

	ue, ok := amfSelf.AmfUeFindByPolicyAssociationID(polAssoID)
	if !ok {
		problemDetails := utils.ProblemDetailsContextNotFound(fmt.Sprintf("Policy Association ID[%s] Not Found", polAssoID))
		return problemDetails
	}

	logger.CallbackLog.Infof("Cause of AM Policy termination[%+v]", terminationNotification.Cause)

	// use go routine to write response first to ensure the order of the procedure
	go func() {
		problem, err := consumer.AMPolicyControlDelete(ctx, ue)
		if problem != nil {
			logger.ProducerLog.Errorf("AM Policy Control Delete Failed Problem[%+v]", problem)
		} else if err != nil {
			logger.ProducerLog.Errorf("AM Policy Control Delete Error[%v]", err.Error())
		}
	}()
	return nil
}

// TS 23.502 4.2.2.2.3 Registration with AMF re-allocation
func HandleN1MessageNotify(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ProducerLog.Infoln("[AMF] handle N1 Message Notify")

	n1MessageNotify := request.Body.(models.N1MessageNotifyRequest)

	problemDetails := N1MessageNotifyProcedure(n1MessageNotify)
	if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	} else {
		return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}

func N1MessageNotifyProcedure(n1MessageNotify models.N1MessageNotifyRequest) *models.ProblemDetails {
	logger.ProducerLog.Debugf("n1MessageNotify: %+v", n1MessageNotify)

	amfSelf := context.AMF_Self()

	registrationCtxtContainer := n1MessageNotify.JsonData.GetRegistrationCtxtContainer()
	if reflect.DeepEqual(registrationCtxtContainer.GetUeContext(), models.UeContext{}) {
		problemDetails := utils.ProblemDetailsMandatoryIeMissing("Missing IE [UeContext] in RegistrationCtxtContainer")
		return problemDetails
	}

	ran, ok := amfSelf.AmfRanFindByRanID(*registrationCtxtContainer.RanNodeId.Get())
	if !ok {
		problemDetails := utils.ProblemDetailsMandatoryIeIncorrect(fmt.Sprintf("can not find RAN[RanId: %+v]", *registrationCtxtContainer.RanNodeId.Get()))
		return problemDetails
	}

	go func() {
		var amfUe *context.AmfUe
		ueContext := registrationCtxtContainer.UeContext
		if ueContext.GetSupi() != "" {
			amfUe = amfSelf.NewAmfUe(ueContext.GetSupi())
		} else {
			amfUe = amfSelf.NewAmfUe("")
		}
		amfUe.CopyDataFromUeContextModel(ueContext)

		ranUe := ran.RanUeFindByRanUeNgapID(int64(registrationCtxtContainer.AnN2ApId))

		ranUe.Location = registrationCtxtContainer.GetUserLocation()
		amfUe.Location = registrationCtxtContainer.GetUserLocation()
		ranUe.UeContextRequest = registrationCtxtContainer.GetUeContextRequest()
		ranUe.OldAmfName = registrationCtxtContainer.InitialAmfName

		if registrationCtxtContainer.AllowedNssai != nil {
			allowedNssai := registrationCtxtContainer.AllowedNssai
			amfUe.AllowedNssai[allowedNssai.AccessType] = allowedNssai.AllowedSnssaiList
		}

		if len(registrationCtxtContainer.ConfiguredNssai) > 0 {
			amfUe.ConfiguredNssai = registrationCtxtContainer.ConfiguredNssai
		}

		amfUe.AttachRanUe(ranUe)

		nasPdu, err := io.ReadAll(n1MessageNotify.GetBinaryDataN1Message())
		if err != nil {
			logger.ProducerLog.Errorf("read N1 Message Failed: %+v", err)
		}
		nas.HandleNAS(ctxt.Background(), ranUe, ngapType.ProcedureCodeInitialUEMessage, nasPdu)
	}()
	return nil
}

func HandleNfSubscriptionStatusNotify(ctx ctxt.Context, request *httpwrapper.Request) *httpwrapper.Response {
	logger.ProducerLog.Debugln("[AMF] handle NF Status Notify")

	notificationData := request.Body.(models.NotificationData)

	problemDetails := NfSubscriptionStatusNotifyProcedure(ctx, notificationData)
	if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	} else {
		return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
	}
}

func NfSubscriptionStatusNotifyProcedure(ctx ctxt.Context, notificationData models.NotificationData) *models.ProblemDetails {
	logger.ProducerLog.Debugf("NfSubscriptionStatusNotify: %+v", notificationData)

	if notificationData.Event == "" || notificationData.NfInstanceUri == "" {
		problemDetails := utils.ProblemDetailsMandatoryIeMissing("Missing IE [Event]/[NfInstanceUri] in NotificationData")
		return problemDetails
	}
	nfInstanceId := notificationData.NfInstanceUri[strings.LastIndex(notificationData.NfInstanceUri, "/")+1:]

	logger.ProducerLog.Infof("Received Subscription Status Notification from NRF: %v", notificationData.Event)
	// If nrf caching is enabled, go ahead and delete the entry from the cache.
	// This will force the amf to do nf discovery and get the updated nf profile from the nrf.
	if notificationData.Event == models.NOTIFICATIONEVENTTYPE_NF_DEREGISTERED {
		if context.AMF_Self().EnableNrfCaching {
			ok := nrfCache.RemoveNfProfileFromNrfCache(nfInstanceId)
			logger.ProducerLog.Debugf("nfinstance %v deleted from cache: %v", nfInstanceId, ok)
		}
		if subscriptionId, ok := context.AMF_Self().NfStatusSubscriptions.Load(nfInstanceId); ok {
			logger.ConsumerLog.Debugf("SubscriptionId of nfInstance %v is %v", nfInstanceId, subscriptionId.(string))
			problemDetails, err := consumer.SendRemoveSubscription(ctx, subscriptionId.(string))
			if problemDetails != nil {
				logger.ConsumerLog.Errorf("Remove NF Subscription Failed Problem[%+v]", problemDetails)
			} else if err != nil {
				logger.ConsumerLog.Errorf("Remove NF Subscription Error[%+v]", err)
			} else {
				logger.ConsumerLog.Infoln("[AMF] Remove NF Subscription successful")
				context.AMF_Self().NfStatusSubscriptions.Delete(nfInstanceId)
			}
		} else {
			logger.ProducerLog.Infof("nfinstance %v not found in map", nfInstanceId)
		}
	}

	return nil
}

func HandleDeregistrationNotification(ctx ctxt.Context, request *httpwrapper.Request) *httpwrapper.Response {
	logger.ProducerLog.Infoln("handle Deregistration Notification")
	deregistrationData := request.Body.(models.DeregistrationData)

	switch deregistrationData.DeregReason {
	case "SUBSCRIPTION_WITHDRAWN":
		amfSelf := context.AMF_Self()
		if supi, exists := request.Params["supi"]; exists {
			reqUri := request.URL.RequestURI()
			if ue, ok := amfSelf.AmfUeFindBySupi(supi); ok {
				logger.ProducerLog.Debugln("amf ue found: ", ue.GetSupi())
				sbiMsg := context.SbiMsg{
					UeContextId: ue.GetSupi(),
					ReqUri:      reqUri,
					Msg:         nil,
					Result:      make(chan context.SbiResponseMsg, 10),
				}
				ue.EventChannel.UpdateSbiHandler(HandleOAMPurgeUEContextRequest)
				ue.EventChannel.SubmitMessage(sbiMsg)
				msg := <-sbiMsg.Result
				if msg.ProblemDetails != nil {
					return httpwrapper.NewResponse(int(msg.ProblemDetails.(*models.ProblemDetails).GetStatus()), nil, msg.ProblemDetails.(*models.ProblemDetails))
				} else {
					return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
				}
			} else {
				return httpwrapper.NewResponse(http.StatusNotFound, nil, nil)
			}
		}

	case "":
		problemDetails := utils.ProblemDetailsMandatoryIeMissing("Missing IE [DeregReason] in DeregistrationData")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)

	default:
		problemDetails := utils.ProblemDetailsNotImplemented("Unsupported [DeregReason] in DeregistrationData")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	return nil
}
