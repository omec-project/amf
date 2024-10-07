// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/openapi/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/models"
	nrfCache "github.com/omec-project/openapi/nrfcache"
)

func SendSearchNFInstances(nrfUri string, targetNfType, requestNfType models.NfType,
	param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts,
) (models.SearchResult, error) {
	if amf_context.AMF_Self().EnableNrfCaching {
		return nrfCache.SearchNFInstances(nrfUri, targetNfType, requestNfType, param)
	} else {
		return SendNfDiscoveryToNrf(nrfUri, targetNfType, requestNfType, param)
	}
}

func SendNfDiscoveryToNrf(nrfUri string, targetNfType, requestNfType models.NfType,
	param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts,
) (models.SearchResult, error) {
	// Set client and set url
	configuration := Nnrf_NFDiscovery.NewConfiguration()
	configuration.SetBasePath(nrfUri)
	client := Nnrf_NFDiscovery.NewAPIClient(configuration)

	result, res, err := client.NFInstancesStoreApi.SearchNFInstances(context.TODO(), targetNfType, requestNfType, param)
	if res != nil && res.StatusCode == http.StatusTemporaryRedirect {
		err = fmt.Errorf("temporary Redirect For Non NRF Consumer")
	}
	defer func() {
		if bodyCloseErr := res.Body.Close(); bodyCloseErr != nil {
			err = fmt.Errorf("SearchNFInstances' response body cannot close: %+w", bodyCloseErr)
		}
	}()

	amfSelf := amf_context.AMF_Self()

	var nrfSubData models.NrfSubscriptionData
	var problemDetails *models.ProblemDetails
	for _, nfProfile := range result.NfInstances {
		// checking whether the AMF subscribed to this target nfinstanceid or not
		if _, ok := amfSelf.NfStatusSubscriptions.Load(nfProfile.NfInstanceId); !ok {
			nrfSubscriptionData := models.NrfSubscriptionData{
				NfStatusNotificationUri: fmt.Sprintf("%s/namf-callback/v1/nf-status-notify", amfSelf.GetIPv4Uri()),
				SubscrCond:              &models.NfInstanceIdCond{NfInstanceId: nfProfile.NfInstanceId},
				ReqNfType:               requestNfType,
			}
			nrfSubData, problemDetails, err = SendCreateSubscription(nrfUri, nrfSubscriptionData)
			if problemDetails != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription to NRF, Problem[%+v]", problemDetails)
			} else if err != nil {
				logger.ConsumerLog.Errorf("SendCreateSubscription Error[%+v]", err)
			}
			amfSelf.NfStatusSubscriptions.Store(nfProfile.NfInstanceId, nrfSubData.SubscriptionId)
		}
	}

	return result, err
}

func SearchUdmSdmInstance(ue *amf_context.AmfUe, nrfUri string, targetNfType, requestNfType models.NfType,
	param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts,
) error {
	if ue.NudmSDMUri != "" {
		return nil
	}

	resp, localErr := SendSearchNFInstances(nrfUri, targetNfType, requestNfType, param)
	if localErr != nil {
		return localErr
	}

	// select the first UDM_SDM, TODO: select base on other info
	nfInstanceIds := make([]string, 0, len(resp.NfInstances))
	for _, nfProfile := range resp.NfInstances {
		nfInstanceIds = append(nfInstanceIds, nfProfile.NfInstanceId)
	}
	sort.Strings(nfInstanceIds)
	nfInstanceIdIndexMap := make(map[string]int)
	for index, value := range nfInstanceIds {
		nfInstanceIdIndexMap[value] = index
	}

	nfInstanceIndex := 0
	if amf_context.AMF_Self().EnableScaling == true {
		// h := fnv.New32a()
		// h.Write([]byte(ue.Supi))
		// // logger.ConsumerLog.Warnln("SearchUdmSdmInstance: ue.Supi: ", ue.Supi)
		// key := int(h.Sum32())
		// nfInstanceIndex = int(key % len(resp.NfInstances))
		parts := strings.Split(ue.Supi, "-")
		imsiNumber, _ := strconv.Atoi(parts[1])
		nfInstanceIndex = imsiNumber % len(resp.NfInstances)
	}
	var sdmUri string
	for _, nfProfile := range resp.NfInstances {
		if nfInstanceIndex != nfInstanceIdIndexMap[nfProfile.NfInstanceId] {
			continue
		}
		ue.UdmId = nfProfile.NfInstanceId
		sdmUri = util.SearchNFServiceUri(nfProfile, models.ServiceName_NUDM_SDM, models.NfServiceStatus_REGISTERED)
		if sdmUri != "" {
			logger.ConsumerLog.Warnln("for Ue: ", ue.Supi, " nfInstanceIndex: ", nfInstanceIndex, " for targetNfType ", string(targetNfType), " NF is: ", nfProfile.Ipv4Addresses)
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

func SearchNssfNSSelectionInstance(ue *amf_context.AmfUe, nrfUri string, targetNfType, requestNfType models.NfType,
	param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts,
) error {
	resp, localErr := SendSearchNFInstances(nrfUri, targetNfType, requestNfType, param)
	if localErr != nil {
		return localErr
	}

	// select the first NSSF, TODO: select base on other info
	var nssfUri string
	nfInstanceIds := make([]string, 0, len(resp.NfInstances))
	for _, nfProfile := range resp.NfInstances {
		nfInstanceIds = append(nfInstanceIds, nfProfile.NfInstanceId)
	}
	sort.Strings(nfInstanceIds)
	nfInstanceIdIndexMap := make(map[string]int)
	for index, value := range nfInstanceIds {
		nfInstanceIdIndexMap[value] = index
	}

	nfInstanceIndex := 0
	if amf_context.AMF_Self().EnableScaling == true {

		parts := strings.Split(ue.Supi, "-")
		imsiNumber, _ := strconv.Atoi(parts[1])
		nfInstanceIndex = imsiNumber % len(resp.NfInstances)
	}
	for _, nfProfile := range resp.NfInstances {
		ue.NssfId = nfProfile.NfInstanceId
		nssfUri = util.SearchNFServiceUri(nfProfile, models.ServiceName_NNSSF_NSSELECTION, models.NfServiceStatus_REGISTERED)
		if nssfUri != "" {
			logger.ConsumerLog.Warnln("for Ue: ", ue.Supi, " nfInstanceIndex: ", nfInstanceIndex, " for targetNfType ", string(targetNfType), " nssfUri:", nssfUri, " NF is: ", nfProfile)
			break
		}
	}
	ue.NssfUri = nssfUri
	if ue.NssfUri == "" {
		return fmt.Errorf("AMF can not select an NSSF by NRF")
	}
	return nil
}

func SearchAmfCommunicationInstance(ue *amf_context.AmfUe, nrfUri string, targetNfType,
	requestNfType models.NfType, param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts,
) (err error) {
	resp, localErr := SendSearchNFInstances(nrfUri, targetNfType, requestNfType, param)
	if localErr != nil {
		err = localErr
		return
	}

	// select the first AMF, TODO: select base on other info
	var amfUri string
	for _, nfProfile := range resp.NfInstances {
		ue.TargetAmfProfile = &nfProfile
		amfUri = util.SearchNFServiceUri(nfProfile, models.ServiceName_NAMF_COMM, models.NfServiceStatus_REGISTERED)
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
