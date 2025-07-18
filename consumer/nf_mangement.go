// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/openapi"
	"github.com/omec-project/openapi/Nnrf_NFManagement"
	"github.com/omec-project/openapi/models"
	"go.opentelemetry.io/otel/attribute"
)

func BuildNFInstance(context *amf_context.AMFContext) (profile models.NfProfile, err error) {
	profile.NfInstanceId = context.NfId
	profile.NfType = models.NfType_AMF
	profile.NfStatus = models.NfStatus_REGISTERED
	var plmns []models.PlmnId
	for _, plmnItem := range context.PlmnSupportList {
		plmns = append(plmns, plmnItem.PlmnId)
	}
	if len(plmns) > 0 {
		profile.PlmnList = &plmns
		// TODO: change to Per Plmn Support Snssai List
		profile.SNssais = &context.PlmnSupportList[0].SNssaiList
	}
	amfInfo := models.AmfInfo{}
	if len(context.ServedGuamiList) == 0 {
		err = fmt.Errorf("gumai List is Empty in AMF")
		return profile, err
	}
	regionId, setId, _, err1 := util.SeperateAmfId(context.ServedGuamiList[0].AmfId)
	if err1 != nil {
		err = err1
		return profile, err
	}
	amfInfo.AmfRegionId = regionId
	amfInfo.AmfSetId = setId
	amfInfo.GuamiList = &context.ServedGuamiList
	if len(context.SupportTaiLists) == 0 {
		err = fmt.Errorf("SupportTaiList is Empty in AMF")
		return profile, err
	}
	amfInfo.TaiList = &context.SupportTaiLists
	profile.AmfInfo = &amfInfo
	if context.RegisterIPv4 == "" {
		err = fmt.Errorf("AMF Address is empty")
		return profile, err
	}
	profile.Ipv4Addresses = append(profile.Ipv4Addresses, context.RegisterIPv4)
	service := []models.NfService{}
	for _, nfService := range context.NfService {
		service = append(service, nfService)
	}
	if len(service) > 0 {
		profile.NfServices = &service
	}

	defaultNotificationSubscription := models.DefaultNotificationSubscription{
		CallbackUri:      fmt.Sprintf("%s/namf-callback/v1/n1-message-notify", context.GetIPv4Uri()),
		NotificationType: models.NotificationType_N1_MESSAGES,
		N1MessageClass:   models.N1MessageClass__5_GMM,
	}
	profile.DefaultNotificationSubscriptions = append(profile.DefaultNotificationSubscriptions, defaultNotificationSubscription)
	return profile, err
}

var SendRegisterNFInstance = func(ctx context.Context, nrfUri, nfInstanceId string, profile models.NfProfile) (
	prof models.NfProfile, resouceNrfUri string, retrieveNfInstanceId string, err error,
) {
	ctx, span := tracer.Start(ctx, "HTTP PUT nrf/nf-instances/{nfInstanceID}")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "PUT"),
		attribute.String("nf.target", "nrf"),
		attribute.String("net.peer.name", nrfUri),
		attribute.String("amf.nf.id", nfInstanceId),
		attribute.String("amf.nf.type", string(profile.NfType)),
	)

	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(nrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	var res *http.Response
	for {
		prof, res, err = client.NFInstanceIDDocumentApi.RegisterNFInstance(ctx, nfInstanceId, profile)
		if err != nil || res == nil {
			logger.ConsumerLog.Errorf("AMF register to NRF Error[%s]", err.Error())
			time.Sleep(2 * time.Second)
			continue
		}
		defer func() {
			if bodyCloseErr := res.Body.Close(); bodyCloseErr != nil {
				err = fmt.Errorf("SearchNFInstances' response body cannot close: %+w", bodyCloseErr)
			}
		}()
		status := res.StatusCode
		if status == http.StatusOK {
			// NFUpdate
			break
		} else if status == http.StatusCreated {
			// NFRegister
			resourceUri := res.Header.Get("Location")
			resouceNrfUri = resourceUri[:strings.Index(resourceUri, "/nnrf-nfm/")]
			retrieveNfInstanceId = resourceUri[strings.LastIndex(resourceUri, "/")+1:]
			break
		} else {
			logger.ConsumerLog.Errorf("handler returned wrong status code %d", status)
			logger.ConsumerLog.Errorf("NRF return wrong status code %d", status)
		}
	}
	return prof, resouceNrfUri, retrieveNfInstanceId, err
}

func SendDeregisterNFInstance(ctx context.Context) (problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Infof("[AMF] Send Deregister NFInstance")

	ctx, span := tracer.Start(ctx, "HTTP DELETE nrf/nf-instances/{nfInstanceID}")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "DELETE"),
		attribute.String("nf.target", "nrf"),
		attribute.String("net.peer.name", amf_context.AMF_Self().NrfUri),
		attribute.String("amf.nf.id", amf_context.AMF_Self().NfId),
	)

	amfSelf := amf_context.AMF_Self()
	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(amfSelf.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	var res *http.Response

	res, err = client.NFInstanceIDDocumentApi.DeregisterNFInstance(ctx, amfSelf.NfId)

	if err == nil {
		return problemDetails, err
	} else if res != nil {
		defer func() {
			if bodyCloseErr := res.Body.Close(); bodyCloseErr != nil {
				err = fmt.Errorf("SearchNFInstances' response body cannot close: %+w", bodyCloseErr)
			}
		}()
		if res.Status != err.Error() {
			return problemDetails, err
		}
		problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("server no response")
	}
	return problemDetails, err
}

var SendUpdateNFInstance = func(patchItem []models.PatchItem) (nfProfile models.NfProfile, problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Debugf("Send Update NFInstance")

	amfSelf := amf_context.AMF_Self()
	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(amfSelf.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	var res *http.Response
	nfProfile, res, err = client.NFInstanceIDDocumentApi.UpdateNFInstance(context.Background(), amfSelf.NfId, patchItem)
	if err == nil {
		return
	} else if res != nil {
		defer func() {
			if resCloseErr := res.Body.Close(); resCloseErr != nil {
				logger.ConsumerLog.Errorf("UpdateNFInstance response cannot close: %+v", resCloseErr)
			}
		}()
		if res.Status != err.Error() {
			logger.ConsumerLog.Errorf("UpdateNFInstance received error response: %v", res.Status)
			return
		}
		problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("server no response")
	}
	return
}

func SendCreateSubscription(ctx context.Context, nrfUri string, nrfSubscriptionData models.NrfSubscriptionData) (nrfSubData models.NrfSubscriptionData, problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Debugf("Send Create Subscription")

	ctx, span := tracer.Start(ctx, "HTTP POST nrf/subscriptions")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "nrf"),
		attribute.String("net.peer.name", nrfUri),
		attribute.String("amf.nf.id", amf_context.AMF_Self().NfId),
	)

	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(nrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	var res *http.Response
	nrfSubData, res, err = client.SubscriptionsCollectionApi.CreateSubscription(ctx, nrfSubscriptionData)
	if err == nil {
		return nrfSubData, problemDetails, err
	} else if res != nil {
		defer func() {
			if resCloseErr := res.Body.Close(); resCloseErr != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription response cannot close: %+v", resCloseErr)
			}
		}()
		if res.Status != err.Error() {
			logger.ConsumerLog.Errorf("SendCreateSubscription received error response: %v", res.Status)
			return nrfSubData, problemDetails, err
		}
		problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("server no response")
	}
	return nrfSubData, problemDetails, err
}

func SendRemoveSubscription(ctx context.Context, subscriptionId string) (problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Infoln("[AMF] Send Remove Subscription")

	ctx, span := tracer.Start(ctx, "HTTP DELETE nrf/subscriptions/{subscriptionID}")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "DELETE"),
		attribute.String("nf.target", "nrf"),
		attribute.String("net.peer.name", amf_context.AMF_Self().NrfUri),
		attribute.String("amf.nf.id", amf_context.AMF_Self().NfId),
	)

	amfSelf := amf_context.AMF_Self()
	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(amfSelf.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)
	var res *http.Response

	res, err = client.SubscriptionIDDocumentApi.RemoveSubscription(ctx, subscriptionId)
	if err == nil {
		return problemDetails, nil
	} else if res != nil {
		defer func() {
			if bodyCloseErr := res.Body.Close(); bodyCloseErr != nil {
				err = fmt.Errorf("RemoveSubscription' response body cannot close: %+w", bodyCloseErr)
			}
		}()
		if res.Status != err.Error() {
			return problemDetails, err
		}
		problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("server no response")
	}
	return problemDetails, err
}
