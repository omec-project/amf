// Copyright 2019 free5GC.org
// SPDX-FileCopyrightText: 2025 Canonical Ltd.
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	amfContext "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/openapi"
	"github.com/omec-project/openapi/Nnrf_NFManagement"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/openapi/nfConfigApi"
	"go.opentelemetry.io/otel/attribute"
)

func getNfProfile(amfCtx *amfContext.AMFContext, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) (profile models.NFProfile, err error) {
	if amfCtx == nil {
		return profile, fmt.Errorf("amf context has not been intialized. NF profile cannot be built")
	}
	newSupportedTais, newPlmnSnssai, newGuamiList := amfContext.ConvertAccessAndMobilityList(accessAndMobilityConfig)
	profile.NfInstanceId = amfCtx.NfId
	profile.NfType = models.NFTYPE_AMF
	profile.NfStatus = models.NFSTATUS_REGISTERED
	plmns := make([]models.PlmnId, len(accessAndMobilityConfig))
	for _, accessAndMobilityData := range accessAndMobilityConfig {
		nfPlmn := models.PlmnId{
			Mcc: accessAndMobilityData.PlmnId.GetMcc(),
			Mnc: accessAndMobilityData.PlmnId.GetMnc(),
		}
		plmns = append(plmns, nfPlmn)
	}
	profile.PlmnList = plmns
	perPlmnSnssaiList := []models.PlmnSnssai{}
	for _, plmnSnssai := range newPlmnSnssai {
		perPlmnSnssai := models.PlmnSnssai{
			PlmnId:     plmnSnssai.PlmnId,
			SNssaiList: plmnSnssai.SNssaiList,
		}
		perPlmnSnssaiList = append(perPlmnSnssaiList, perPlmnSnssai)
	}
	profile.PerPlmnSnssaiList = perPlmnSnssaiList
	var amfInfo models.AmfInfo
	if len(newGuamiList) == 0 {
		err = fmt.Errorf("guami list is empty in AMF")
		return profile, err
	}
	regionId, setId, _, err := util.SeparateAmfId(newGuamiList[0].AmfId)
	if err != nil {
		return profile, err
	}
	amfInfo.AmfRegionId = regionId
	amfInfo.AmfSetId = setId
	amfInfo.GuamiList = newGuamiList
	if len(newSupportedTais) == 0 {
		err = fmt.Errorf("SupportTaiList is empty in AMF")
		return profile, err
	}
	amfInfo.TaiList = newSupportedTais
	profile.AmfInfo = &amfInfo
	if amfCtx.RegisterIPv4 == "" {
		err = fmt.Errorf("AMF Address is empty")
		return profile, err
	}
	profile.Ipv4Addresses = append(profile.Ipv4Addresses, amfCtx.RegisterIPv4)
	services := []models.NFService{}
	for _, nfService := range amfCtx.NfService {
		services = append(services, nfService)
	}
	if len(services) > 0 {
		profile.NfServices = services
	}

	defaultNotificationSubscription := models.DefaultNotificationSubscription{
		CallbackUri:      fmt.Sprintf("%s/namf-callback/v1/n1-message-notify", amfCtx.GetIPv4Uri()),
		NotificationType: models.NOTIFICATIONTYPE_N1_MESSAGES,
		N1MessageClass:   models.N1MESSAGECLASS__5_GMM.Ptr(),
	}
	profile.DefaultNotificationSubscriptions = append(profile.DefaultNotificationSubscriptions, defaultNotificationSubscription)
	return profile, nil
}

var SendRegisterNFInstance = func(ctx context.Context, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) (prof *models.NFProfile, resourceNrfUri string, err error) {
	self := amfContext.AMF_Self()
	nfProfile, err := getNfProfile(self, accessAndMobilityConfig)
	if err != nil {
		return &models.NFProfile{}, "", err
	}

	ctx, span := tracer.Start(ctx, "HTTP PUT nrf/nf-instances/{nfInstanceID}")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "PUT"),
		attribute.String("nf.target", "nrf"),
		attribute.String("net.peer.name", self.NrfUri),
		attribute.String("amf.nf.id", nfProfile.NfInstanceId),
		attribute.String("amf.nf.type", string(nfProfile.NfType)),
	)

	configuration := Nnrf_NFManagement.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = self.NrfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nnrf_NFManagement.NewAPIClient(configuration)
	apiRegisterNFInstanceRequest := client.NFInstanceIDDocumentAPI.RegisterNFInstance(ctx, nfProfile.NfInstanceId)
	apiRegisterNFInstanceRequest = apiRegisterNFInstanceRequest.NFProfile(nfProfile)
	receivedNfProfile, res, err := client.NFInstanceIDDocumentAPI.RegisterNFInstanceExecute(apiRegisterNFInstanceRequest)
	if err != nil {
		return &models.NFProfile{}, "", err
	}
	if res == nil {
		return &models.NFProfile{}, "", fmt.Errorf("no response from server")
	}

	switch res.StatusCode {
	case http.StatusOK: // NFUpdate
		logger.ConsumerLog.Debugln("AMF NF profile updated with complete replacement")
		return receivedNfProfile, "", nil
	case http.StatusCreated: // NFRegister
		resourceUri := res.Header.Get("Location")
		resourceNrfUri = resourceUri[:strings.Index(resourceUri, "/nnrf-nfm/")]
		retrieveNfInstanceId := resourceUri[strings.LastIndex(resourceUri, "/")+1:]
		self.NfId = retrieveNfInstanceId
		logger.ConsumerLog.Debugln("AMF NF profile registered to the NRF")
		return receivedNfProfile, resourceNrfUri, nil
	default:
		return receivedNfProfile, "", fmt.Errorf("NRF returned unexpected status code %d", res.StatusCode)
	}
}

var SendDeregisterNFInstance = func(ctx context.Context) error {
	logger.ConsumerLog.Infoln("send Deregister NFInstance")

	amfSelf := amfContext.AMF_Self()

	ctx, span := tracer.Start(ctx, "HTTP DELETE nrf/nf-instances/{nfInstanceID}")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "DELETE"),
		attribute.String("nf.target", "nrf"),
		attribute.String("net.peer.name", amfSelf.NrfUri),
		attribute.String("amf.nf.id", amfSelf.NfId),
	)
	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = amfSelf.NrfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	apiDeregisterNFInstanceRequest := client.NFInstanceIDDocumentAPI.DeregisterNFInstance(ctx, amfSelf.NfId)
	res, err := client.NFInstanceIDDocumentAPI.DeregisterNFInstanceExecute(apiDeregisterNFInstanceRequest)
	if err != nil {
		return err
	}
	if res == nil {
		return fmt.Errorf("no response from server")
	}
	if res.StatusCode == http.StatusNoContent {
		return nil
	}
	return fmt.Errorf("unexpected response code")
}

var SendUpdateNFInstance = func(patchItem []models.PatchItem) (receivedNfProfile *models.NFProfile, problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Debugln("send Update NFInstance")

	amfSelf := amfContext.AMF_Self()
	configuration := Nnrf_NFManagement.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = amfSelf.NrfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	var res *http.Response
	apiUpdateNFInstanceRequest := client.NFInstanceIDDocumentAPI.UpdateNFInstance(context.Background(), amfSelf.NfId)
	apiUpdateNFInstanceRequest = apiUpdateNFInstanceRequest.PatchItem(patchItem)
	receivedNfProfile, res, err = client.NFInstanceIDDocumentAPI.UpdateNFInstanceExecute(apiUpdateNFInstanceRequest)
	if err != nil {
		if openapiErr, ok := openapi.AsGenericOpenAPIError(err); ok {
			if model := openapiErr.Model(); model != nil {
				if problem, ok := model.(models.ProblemDetails); ok {
					return &models.NFProfile{}, &problem, nil
				}
			}
		}
		return &models.NFProfile{}, nil, err
	}

	if res == nil {
		return &models.NFProfile{}, nil, fmt.Errorf("no response from server")
	}
	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusNoContent {
		return receivedNfProfile, nil, nil
	}
	return &models.NFProfile{}, nil, fmt.Errorf("unexpected response code")
}

var SendCreateSubscription = func(ctx context.Context, nrfUri string, nrfSubscriptionData models.SubscriptionData) (nrfSubData *models.SubscriptionData, problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Debugln("send Create Subscription")

	ctx, span := tracer.Start(ctx, "HTTP POST nrf/subscriptions")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "nrf"),
		attribute.String("net.peer.name", nrfUri),
		attribute.String("amf.nf.id", amfContext.AMF_Self().NfId),
	)

	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = nrfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	var res *http.Response
	apiCreateSubscriptionRequest := client.SubscriptionsCollectionAPI.CreateSubscription(ctx)
	apiCreateSubscriptionRequest = apiCreateSubscriptionRequest.SubscriptionData(nrfSubscriptionData)
	nrfSubData, res, err = client.SubscriptionsCollectionAPI.CreateSubscriptionExecute(apiCreateSubscriptionRequest)
	if err == nil {
		return nrfSubData, problemDetails, err
	} else if res != nil {
		defer func() {
			if resCloseErr := res.Body.Close(); resCloseErr != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription response cannot close: %+v", resCloseErr)
			}
		}()
		if res.Status != err.Error() {
			logger.ConsumerLog.Errorf("SendCreateSubscription received error response: %s", res.Status)
			return nrfSubData, problemDetails, err
		}
		if problem, ok := openapi.ErrorModel[models.ProblemDetails](err); ok {
			problemDetails = &problem
		} else {
			return nrfSubData, problemDetails, err
		}
	} else {
		err = fmt.Errorf("server no response")
	}
	return nrfSubData, problemDetails, err
}

var SendRemoveSubscription = func(ctx context.Context, subscriptionId string) (problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Infoln("send Remove Subscription")

	amfSelf := amfContext.AMF_Self()

	ctx, span := tracer.Start(ctx, "HTTP DELETE nrf/subscriptions/{subscriptionID}")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "DELETE"),
		attribute.String("nf.target", "nrf"),
		attribute.String("net.peer.name", amfSelf.NrfUri),
		attribute.String("amf.nf.id", amfSelf.NfId),
	)

	// Set client and set url
	configuration := Nnrf_NFManagement.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = amfSelf.NrfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nnrf_NFManagement.NewAPIClient(configuration)
	var res *http.Response

	apiRemoveSubscriptionRequest := client.SubscriptionIDDocumentAPI.RemoveSubscription(ctx, subscriptionId)
	res, err = client.SubscriptionIDDocumentAPI.RemoveSubscriptionExecute(apiRemoveSubscriptionRequest)
	if err == nil {
		return problemDetails, nil
	} else if res != nil {
		defer func() {
			if bodyCloseErr := res.Body.Close(); bodyCloseErr != nil {
				err = fmt.Errorf("RemoveSubscription's response body cannot close: %w", bodyCloseErr)
			}
		}()
		if res.Status != err.Error() {
			return problemDetails, err
		}
		if problem, ok := openapi.ErrorModel[models.ProblemDetails](err); ok {
			problemDetails = &problem
		} else {
			return problemDetails, err
		}
	} else {
		err = fmt.Errorf("server no response")
	}
	return problemDetails, err
}
