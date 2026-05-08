// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"fmt"
	"net/http"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/openapi"
	"github.com/omec-project/openapi/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/models"
	nrfCache "github.com/omec-project/openapi/nrfcache"
	"go.opentelemetry.io/otel/attribute"
)

func SendSearchNFInstances(ctx context.Context, nrfUri string, targetNfType, requestNfType models.NFType,
	param Nnrf_NFDiscovery.ApiSearchNFInstancesRequest,
) (*models.SearchResult, error) {
	param = param.TargetNfType(targetNfType)
	param = param.RequesterNfType(requestNfType)

	if amf_context.AMF_Self().EnableNrfCaching {
		return nrfCache.SearchNFInstances(ctx, nrfUri, targetNfType, requestNfType, param)
	} else {
		return SendNfDiscoveryToNrf(ctx, nrfUri, targetNfType, requestNfType, param)
	}
}

func SendNfDiscoveryToNrf(ctx context.Context, nrfUri string, targetNfType, requestNfType models.NFType,
	param Nnrf_NFDiscovery.ApiSearchNFInstancesRequest,
) (*models.SearchResult, error) {
	ctx, span := tracer.Start(ctx, "HTTP GET nrf/nf-instances")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "GET"),
		attribute.String("nf.target", "nrf"),
		attribute.String("net.peer.name", nrfUri),
		attribute.String("amf.nf.id", amf_context.AMF_Self().NfId),
		attribute.String("request.nf.type", string(requestNfType)),
	)

	// Set client and set url
	configuration := Nnrf_NFDiscovery.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = nrfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nnrf_NFDiscovery.NewAPIClient(configuration)
	param = param.TargetNfType(targetNfType)
	param = param.RequesterNfType(requestNfType)

	result, res, err := client.NFInstancesStoreAPI.SearchNFInstancesExecute(param)
	if res != nil && res.StatusCode == http.StatusTemporaryRedirect {
		err = fmt.Errorf("temporary Redirect For Non NRF Consumer")
	}
	defer func() {
		if res == nil || res.Body == nil {
			return
		}
		if bodyCloseErr := res.Body.Close(); bodyCloseErr != nil && err == nil {
			err = fmt.Errorf("SearchNFInstances' response body cannot close: %+w", bodyCloseErr)
		}
	}()

	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("search nf instances returned no result")
	}

	amfSelf := amf_context.AMF_Self()

	for _, nfProfile := range result.NfInstances {
		// checking whether the AMF subscribed to this target nfinstanceid or not
		if _, ok := amfSelf.NfStatusSubscriptions.Load(nfProfile.NfInstanceId); !ok {
			nrfSubscriptionData := models.SubscriptionData{
				NfStatusNotificationUri: fmt.Sprintf("%s/namf-callback/v1/nf-status-notify", amfSelf.GetIPv4Uri()),
				SubscrCond: &models.SubscrCond{
					NfInstanceIdCond: &models.NfInstanceIdCond{
						NfInstanceId: openapi.PtrString(nfProfile.NfInstanceId),
					},
				},
				ReqNfType: &requestNfType,
			}
			nrfSubData, problemDetails, err1 := SendCreateSubscription(ctx, nrfUri, nrfSubscriptionData)
			if problemDetails != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription to NRF, Problem[%+v]", problemDetails)
			} else if err1 != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription Error[%+v]", err1)
			} else if nrfSubData != nil {
				amfSelf.NfStatusSubscriptions.Store(nfProfile.GetNfInstanceId(), nrfSubData.GetSubscriptionId())
			}
		}
	}

	return result, err
}

func SearchUdmSdmInstance(ctx context.Context, ue *amf_context.AmfUe, nrfUri string, targetNfType, requestNfType models.NFType,
	param Nnrf_NFDiscovery.ApiSearchNFInstancesRequest,
) error {
	resp, localErr := SendSearchNFInstances(ctx, nrfUri, targetNfType, requestNfType, param)
	if localErr != nil {
		return localErr
	}

	// select the first UDM_SDM, TODO: select base on other info
	var sdmUri string
	for _, nfProfile := range resp.NfInstances {
		ue.UdmId = nfProfile.NfInstanceId
		sdmUri = util.SearchNFServiceUri(nfProfile, models.SERVICENAME_NUDM_SDM, models.NFSERVICESTATUS_REGISTERED)
		if sdmUri != "" {
			break
		}
	}
	ue.NudmSDMUri = sdmUri
	if ue.NudmSDMUri == "" {
		err := fmt.Errorf("AMF can not select an UDM by NRF")
		logger.ConsumerLog.Errorln(err.Error())
		return err
	}
	return nil
}

func SearchNssfNSSelectionInstance(ctx context.Context, ue *amf_context.AmfUe, nrfUri string, targetNfType, requestNfType models.NFType,
	param Nnrf_NFDiscovery.ApiSearchNFInstancesRequest,
) error {
	resp, localErr := SendSearchNFInstances(ctx, nrfUri, targetNfType, requestNfType, param)
	if localErr != nil {
		return localErr
	}

	// select the first NSSF, TODO: select base on other info
	var nssfUri string
	for _, nfProfile := range resp.NfInstances {
		ue.NssfId = nfProfile.NfInstanceId
		nssfUri = util.SearchNFServiceUri(nfProfile, models.SERVICENAME_NNSSF_NSSELECTION, models.NFSERVICESTATUS_REGISTERED)
		if nssfUri != "" {
			break
		}
	}
	ue.NssfUri = nssfUri
	if ue.NssfUri == "" {
		return fmt.Errorf("AMF can not select an NSSF by NRF")
	}
	return nil
}

func SearchAmfCommunicationInstance(ctx context.Context, ue *amf_context.AmfUe, nrfUri string, targetNfType,
	requestNfType models.NFType, param Nnrf_NFDiscovery.ApiSearchNFInstancesRequest,
) (err error) {
	resp, localErr := SendSearchNFInstances(ctx, nrfUri, targetNfType, requestNfType, param)
	if localErr != nil {
		err = localErr
		return
	}

	// select the first AMF, TODO: select base on other info
	var amfUri string
	for _, nfProfile := range resp.NfInstances {
		ue.TargetAmfProfile = &nfProfile
		amfUri = util.SearchNFServiceUri(nfProfile, models.SERVICENAME_NAMF_COMM, models.NFSERVICESTATUS_REGISTERED)
		if amfUri != "" {
			break
		}
	}
	ue.TargetAmfUri = amfUri
	if ue.TargetAmfUri == "" {
		err = fmt.Errorf("AMF can not select an target AMF by NRF")
	}
	return
}
