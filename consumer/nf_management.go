// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd.
// Copyright 2019 free5GC.org
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"
	"fmt"

	amfContext "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/nfConfigApi"
	"github.com/omec-project/openapi/v2/nrfclient"
	"go.opentelemetry.io/otel/attribute"
)

func newNrfClient(nrfUri string) *nrfclient.Client {
	return nrfclient.New(nrfUri, nrfclient.WithLogger(logger.ConsumerLog))
}

func getNfProfile(amfCtx *amfContext.AMFContext, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) (profile models.NFProfile, err error) {
	if amfCtx == nil {
		return profile, openapi.ReportError("amf context has not been initialized. NF profile cannot be built")
	}
	newSupportedTais, newPlmnSnssai, newGuamiList := amfContext.ConvertAccessAndMobilityList(accessAndMobilityConfig)
	profile.SetNfInstanceId(amfCtx.NfId)
	profile.SetNfType(models.NFTYPE_AMF)
	profile.SetNfStatus(models.NFSTATUS_REGISTERED)
	plmns := make([]models.PlmnId, 0, len(accessAndMobilityConfig))
	for _, accessAndMobilityData := range accessAndMobilityConfig {
		nfPlmn := models.PlmnId{
			Mcc: accessAndMobilityData.PlmnId.GetMcc(),
			Mnc: accessAndMobilityData.PlmnId.GetMnc(),
		}
		plmns = append(plmns, nfPlmn)
	}
	profile.SetPlmnList(plmns)
	perPlmnSnssaiList := []models.PlmnSnssai{}
	for _, plmnSnssai := range newPlmnSnssai {
		perPlmnSnssai := models.PlmnSnssai{
			PlmnId:     plmnSnssai.PlmnId,
			SNssaiList: plmnSnssai.SNssaiList,
		}
		perPlmnSnssaiList = append(perPlmnSnssaiList, perPlmnSnssai)
	}
	profile.SetPerPlmnSnssaiList(perPlmnSnssaiList)
	var amfInfo models.AmfInfo
	if len(newGuamiList) == 0 {
		err = openapi.ReportError("guami list is empty in AMF")
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
		err = openapi.ReportError("SupportTaiList is empty in AMF")
		return profile, err
	}
	amfInfo.TaiList = newSupportedTais
	profile.SetAmfInfo(amfInfo)
	if amfCtx.RegisterIPv4 == "" {
		err = openapi.ReportError("AMF Address is empty")
		return profile, err
	}
	if registerIPv4 := amfCtx.RegisterIPv4Address(); registerIPv4 != "" {
		profile.SetIpv4Addresses(append(profile.Ipv4Addresses, registerIPv4))
	} else if fqdn := amfCtx.RegisterFQDN(); fqdn != "" {
		profile.SetFqdn(fqdn)
	}
	services := map[string]models.NFService{}
	serviceList := []models.NFService{}
	for _, nfService := range amfCtx.NfService {
		services[nfService.GetServiceInstanceId()] = nfService
		serviceList = append(serviceList, nfService)
	}
	if len(services) > 0 {
		profile.SetNfServices(serviceList)
		profile.SetNfServiceList(services)
	}

	defaultNotificationSubscription := models.NewDefaultNotificationSubscription(models.NOTIFICATIONTYPE_N1_MESSAGES, fmt.Sprintf("%s/namf-callback/v1/n1-message-notify", amfCtx.GetSbiUri()))
	defaultNotificationSubscription.SetN1MessageClass(models.N1MESSAGECLASS__5_GMM)
	profile.DefaultNotificationSubscriptions = append(profile.DefaultNotificationSubscriptions, *defaultNotificationSubscription)
	return profile, nil
}

var SendRegisterNFInstance = func(ctx context.Context, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) (prof *models.NFProfile, resourceNrfUri string, err error) {
	self := amfContext.AMF_Self()
	nfProfile, err := getNfProfile(self, accessAndMobilityConfig)
	if err != nil {
		return models.NewNFProfileWithDefaults(), "", err
	}

	ctx, span := tracer.Start(ctx, "HTTP PUT nrf/nf-instances/{nfInstanceID}")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "PUT"),
		attribute.String("nf.target", "nrf"),
		attribute.String("net.peer.name", self.NrfUri),
		attribute.String("amf.nf.id", nfProfile.GetNfInstanceId()),
		attribute.String("amf.nf.type", string(nfProfile.GetNfType())),
	)

	result, err := newNrfClient(self.NrfUri).Register(ctx, nfProfile)
	if err != nil {
		return models.NewNFProfileWithDefaults(), "", err
	}
	if result.Created {
		logger.ConsumerLog.Debugln("AMF NF profile registered to the NRF")
		if result.NfInstanceID != "" {
			self.NfId = result.NfInstanceID
		}
	} else {
		logger.ConsumerLog.Debugln("AMF NF profile updated with complete replacement")
	}
	return result.Profile, result.NrfURI, nil
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
	return newNrfClient(amfSelf.NrfUri).Deregister(ctx, amfSelf.NfId)
}

// SendUpdateNFInstance sends the heartbeat. ctx bounds the request: the registration service holds its
// registration mutex while the heartbeat runs, and cancels ctx to end it when a new configuration arrives or
// the NF terminates.
var SendUpdateNFInstance = func(ctx context.Context, patchItem []models.PatchItem) (receivedNfProfile *models.NFProfile, problemDetails *models.ProblemDetails, err error) {
	logger.ConsumerLog.Debugln("send Update NFInstance")

	amfSelf := amfContext.AMF_Self()
	return newNrfClient(amfSelf.NrfUri).Update(ctx, amfSelf.NfId, patchItem)
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

	return newNrfClient(nrfUri).CreateSubscription(ctx, nrfSubscriptionData)
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

	return newNrfClient(amfSelf.NrfUri).RemoveSubscription(ctx, subscriptionId)
}
