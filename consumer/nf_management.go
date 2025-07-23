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

func getNfProfile(amfContext *amfContext.AMFContext, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) (profile models.NfProfile, err error) {
	if amfContext == nil {
		return profile, fmt.Errorf("amf context has not been intialized. NF profile cannot be built")
	}
	profile.NfInstanceId = amfContext.NfId
	profile.NfType = models.NfType_AMF
	profile.NfStatus = models.NfStatus_REGISTERED
	profile.Ipv4Addresses = append(profile.Ipv4Addresses, amfContext.RegisterIPv4)
	services := []models.NfService{}
	for _, nfService := range amfContext.NfService {
		services = append(services, nfService)
	}
	if len(services) > 0 {
		profile.NfServices = &services
	}
	var amfInfo models.AmfInfo
	if len(amfContext.ServedGuamiList) == 0 {
		err = fmt.Errorf("gumai List is Empty in AMF")
		return profile, err
	}
	regionId, setId, _, err1 := util.SeperateAmfId(amfContext.ServedGuamiList[0].AmfId)
	if err1 != nil {
		err = err1
		return profile, err
	}
	amfInfo.AmfRegionId = regionId
	amfInfo.AmfSetId = setId
	amfInfo.GuamiList = &amfContext.ServedGuamiList
	if len(amfContext.SupportTaiLists) == 0 {
		err = fmt.Errorf("SupportTaiList is Empty in AMF")
		return profile, err
	}
	amfInfo.TaiList = &amfContext.SupportTaiLists
	profile.AmfInfo = &amfInfo
	plmnCopy := make([]models.PlmnId, len(accessAndMobilityConfig))
	for _, accessAndMobilityData := range accessAndMobilityConfig {
		nfPlmn := models.PlmnId{
			Mcc: accessAndMobilityData.PlmnId.GetMcc(),
			Mnc: accessAndMobilityData.PlmnId.GetMnc(),
		}
		plmnCopy = append(plmnCopy, nfPlmn)
	}
	profile.PlmnList = &plmnCopy
	return profile, nil
}

func BuildNFInstance(amfContext *amfContext.AMFContext) (profile models.NfProfile, err error) {
	profile.NfInstanceId = amfContext.NfId
	profile.NfType = models.NfType_AMF
	profile.NfStatus = models.NfStatus_REGISTERED
	var plmns []models.PlmnId
	for _, plmnItem := range amfContext.PlmnSupportList {
		plmns = append(plmns, plmnItem.PlmnId)
	}
	if len(plmns) > 0 {
		profile.PlmnList = &plmns
		// TODO: change to Per Plmn Support Snssai List
		profile.SNssais = &amfContext.PlmnSupportList[0].SNssaiList
	}
	amfInfo := models.AmfInfo{}
	if len(amfContext.ServedGuamiList) == 0 {
		err = fmt.Errorf("gumai List is Empty in AMF")
		return profile, err
	}
	regionId, setId, _, err1 := util.SeperateAmfId(amfContext.ServedGuamiList[0].AmfId)
	if err1 != nil {
		err = err1
		return profile, err
	}
	amfInfo.AmfRegionId = regionId
	amfInfo.AmfSetId = setId
	amfInfo.GuamiList = &amfContext.ServedGuamiList
	if len(amfContext.SupportTaiLists) == 0 {
		err = fmt.Errorf("SupportTaiList is Empty in AMF")
		return profile, err
	}
	amfInfo.TaiList = &amfContext.SupportTaiLists
	profile.AmfInfo = &amfInfo
	if amfContext.RegisterIPv4 == "" {
		err = fmt.Errorf("AMF Address is empty")
		return profile, err
	}
	profile.Ipv4Addresses = append(profile.Ipv4Addresses, amfContext.RegisterIPv4)
	service := []models.NfService{}
	for _, nfService := range amfContext.NfService {
		service = append(service, nfService)
	}
	if len(service) > 0 {
		profile.NfServices = &service
	}

	defaultNotificationSubscription := models.DefaultNotificationSubscription{
		CallbackUri:      fmt.Sprintf("%s/namf-callback/v1/n1-message-notify", amfContext.GetIPv4Uri()),
		NotificationType: models.NotificationType_N1_MESSAGES,
		N1MessageClass:   models.N1MessageClass__5_GMM,
	}
	profile.DefaultNotificationSubscriptions = append(profile.DefaultNotificationSubscriptions, defaultNotificationSubscription)
	return profile, err
}

var SendRegisterNFInstance = func(ctx context.Context, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) (prof models.NfProfile, resourceNrfUri string, err error) {
	self := amfContext.AMF_Self()
	nfProfile, err := getNfProfile(self, accessAndMobilityConfig)
	if err != nil {
		return models.NfProfile{}, "", err
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
	configuration.SetBasePath(self.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)
	receivedNfProfile, res, err := client.NFInstanceIDDocumentApi.RegisterNFInstance(ctx, nfProfile.NfInstanceId, nfProfile)
	logger.ConsumerLog.Debugf("RegisterNFInstance done using profile: %+v", nfProfile)

	if err != nil {
		return models.NfProfile{}, "", err
	}
	if res == nil {
		return models.NfProfile{}, "", fmt.Errorf("no response from server")
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
		return receivedNfProfile, "", fmt.Errorf("unexpected status code returned by the NRF %d", res.StatusCode)
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
	configuration.SetBasePath(amfSelf.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	res, err := client.NFInstanceIDDocumentApi.DeregisterNFInstance(ctx, amfSelf.NfId)
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

var SendUpdateNFInstance = func(patchItem []models.PatchItem) (receivedNfProfile models.NfProfile, problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Debugln("send Update NFInstance")

	amfSelf := amfContext.AMF_Self()
	configuration := Nnrf_NFManagement.NewConfiguration()
	configuration.SetBasePath(amfSelf.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)

	var res *http.Response
	receivedNfProfile, res, err = client.NFInstanceIDDocumentApi.UpdateNFInstance(context.Background(), amfSelf.NfId, patchItem)
	if err != nil {
		if openapiErr, ok := err.(openapi.GenericOpenAPIError); ok {
			if model := openapiErr.Model(); model != nil {
				if problem, ok := model.(models.ProblemDetails); ok {
					return models.NfProfile{}, &problem, nil
				}
			}
		}
		return models.NfProfile{}, nil, err
	}

	if res == nil {
		return models.NfProfile{}, nil, fmt.Errorf("no response from server")
	}
	if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusNoContent {
		return receivedNfProfile, nil, nil
	}
	return models.NfProfile{}, nil, fmt.Errorf("unexpected response code")
}

var SendCreateSubscription = func(ctx context.Context, nrfUri string, nrfSubscriptionData models.NrfSubscriptionData) (nrfSubData models.NrfSubscriptionData, problemDetails *models.ProblemDetails, err error) {
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
	configuration.SetBasePath(amfSelf.NrfUri)
	client := Nnrf_NFManagement.NewAPIClient(configuration)
	var res *http.Response

	res, err = client.SubscriptionIDDocumentApi.RemoveSubscription(ctx, subscriptionId)
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
		problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = fmt.Errorf("server no response")
	}
	return problemDetails, err
}
