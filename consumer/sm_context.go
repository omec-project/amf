// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/antihax/optional"
	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/nas/nasMessage"
	"github.com/omec-project/openapi"
	"github.com/omec-project/openapi/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/Nsmf_PDUSession"
	"github.com/omec-project/openapi/models"
)

const N2SMINFO_ID = "N2SmInfo"

// func getServingSmfIndex(smfNum int) (servingSmfIndex int) {
// 	servingSmfIndexStr := os.Getenv("SERVING_SMF_INDEX")
// 	i, err := strconv.Atoi(servingSmfIndexStr)
// 	if err != nil {
// 		logger.ConsumerLog.Errorf("Could not convert %s to int: %v", servingSmfIndexStr, err)
// 	}
// 	servingSmfIndexInt := i + 1
// 	servingSmfIndex = servingSmfIndexInt % smfNum
// 	if err := os.Setenv("SERVING_SMF_INDEX", strconv.Itoa(servingSmfIndex)); err != nil {
// 		logger.ConsumerLog.Errorf("Could not set env SERVING_SMF_INDEX: %v", err)
// 	}
// 	return
// }

func setAltSmfProfile(smCtxt *amf_context.SmContext) error {
	ignoreSmfId := smCtxt.SmfID()
	var altSmfInst []models.NfProfile
	// iterate over nf instances to ignore failed NF
	for _, inst := range smCtxt.SmfProfiles {
		if inst.NfInstanceId != ignoreSmfId {
			altSmfInst = append(altSmfInst, inst)
		}
	}

	if len(altSmfInst) > 0 {
		smCtxt.SmfProfiles = altSmfInst
		nfProfile := altSmfInst[0]
		smfUri := util.SearchNFServiceUri(nfProfile, models.ServiceName_NSMF_PDUSESSION, models.NfServiceStatus_REGISTERED)
		smCtxt.SetSmfID(nfProfile.NfInstanceId)
		smCtxt.SetSmfUri(smfUri)
		return nil
	} else {
		nrfUri := amf_context.AMF_Self().NrfUri
		// fmt.Println("Get NEW SMFS")
		param := Nnrf_NFDiscovery.SearchNFInstancesParamOpts{
			ServiceNames: optional.NewInterface([]models.ServiceName{models.ServiceName_NSMF_PDUSESSION}),
			Dnn:          optional.NewString(smCtxt.Dnn()),
			Snssais:      optional.NewInterface(util.MarshToJsonString([]models.Snssai{smCtxt.Snssai()})),
		}

		result, err := SendSearchNFInstances(nrfUri, models.NfType_SMF, models.NfType_AMF, &param)
		if err != nil {
			// fmt.Println("setAltSmfProfile: Error while getting New SMF Profiles")
			return fmt.Errorf("setAltSmfProfile: Error while getting New SMF Profiles")
		}
		var altSmfInst []models.NfProfile
		//iterate over nf instances to ignore failed NF
		for _, inst := range result.NfInstances {
			if inst.NfInstanceId != ignoreSmfId {
				altSmfInst = append(altSmfInst, inst)
			}
		}

		if len(altSmfInst) == 0 {
			fmt.Println("setAltSmfProfile: No New SMF availabl, try with old one!")
			return nil
		}

		// select the first SMF, TODO: select base on other info
		smCtxt.SmfProfiles = result.NfInstances
		// fmt.Println("setAltSmfProfile: MinLbSMF", amf_context.AMF_Self().MinLbSMF)
		if amf_context.AMF_Self().MinLbSMF {
			// fmt.Println("setAltSmfProfile: Selecting from minLbSMF")
			min_load := math.MaxInt32
			nf := models.NfProfile{}
			for _, nfProfile := range altSmfInst {
				if nfProfile.NfInstanceId != ignoreSmfId {
					// fmt.Println("setAltSmfProfile: nfProfile Load: ", nfProfile.Load, nfProfile)
					if int(nfProfile.Load) < min_load {
						nf = nfProfile
						min_load = int(nfProfile.Load)
					}
				}
			}
			smfUri := util.SearchNFServiceUri(nf, models.ServiceName_NSMF_PDUSESSION, models.NfServiceStatus_REGISTERED)
			smCtxt.SetSmfID(nf.NfInstanceId)
			smCtxt.SetSmfUri(smfUri)
			//TODO: update ue smf uri
			// ue.SmfUri = smfUri
			// ue.SmfNfId = nf.NfInstanceId
			logger.ConsumerLog.Error("setAltSmfProfile: for targetNfType ", string(models.NfType_SMF), " NF is: ", nf.Ipv4Addresses, " Load Count: ", min_load)
			return nil
		} else {
			for _, nfProfile := range altSmfInst {
				if nfProfile.NfInstanceId != ignoreSmfId {
					continue
				}
				smfUri := util.SearchNFServiceUri(nfProfile, models.ServiceName_NSMF_PDUSESSION, models.NfServiceStatus_REGISTERED)
				smCtxt.SetSmfID(nfProfile.NfInstanceId)
				smCtxt.SetSmfUri(smfUri)
				//TODO: update ue smf uri
				// ue.SmfUri = smfUri
				// ue.SmfNfId = nfProfile.NfInstanceId
				logger.ConsumerLog.Warnln("setAltSmfProfile: for targetNfType ", string(models.NfType_SMF), " NF is: ", nfProfile.Ipv4Addresses)
				return nil
			}
		}
	}
	return fmt.Errorf("setAltSmfProfile: no alternate profiles available")
}

// func refreshSmfProfiles(ue *amf_context.AmfUe, smCtxt *amf_context.SmContext, ignoreSmfId string) *[]models.NfProfile {

// 	nrfUri := ue.ServingAMF.NrfUri
// 	param := Nnrf_NFDiscovery.SearchNFInstancesParamOpts{
// 		ServiceNames: optional.NewInterface([]models.ServiceName{models.ServiceName_NSMF_PDUSESSION}),
// 		Dnn:          optional.NewString(smCtxt.Dnn()),
// 		Snssais:      optional.NewInterface(util.MarshToJsonString([]models.Snssai{smCtxt.Snssai()})),
// 	}

// 	result, err := SendSearchNFInstances(nrfUri, models.NfType_SMF, models.NfType_AMF, &param)
// 	if err != nil {
// 		return nil
// 	}
// 	var altSmfInst []models.NfProfile
// 	//iterate over nf instances to ignore failed NF
// 	for _, inst := range result.NfInstances {
// 		if inst.NfInstanceId != ignoreSmfId {
// 			altSmfInst = append(altSmfInst, inst)
// 		}
// 	}
// 	return &altSmfInst
// }

func SelectSmf(
	ue *amf_context.AmfUe,
	anType models.AccessType,
	pduSessionID int32,
	snssai models.Snssai,
	dnn string,
) (*amf_context.SmContext, uint8, error) {
	var smfUri string

	ue.GmmLog.Infof("Select SMF [snssai: %+v, dnn: %+v]", snssai, dnn)

	nrfUri := ue.ServingAMF.NrfUri // default NRF URI is pre-configured by AMF

	nsiInformation := ue.GetNsiInformationFromSnssai(anType, snssai)
	if nsiInformation == nil {
		if ue.NssfUri == "" {
			// TODO: Set a timeout of NSSF Selection or will starvation here
			for {
				if err := SearchNssfNSSelectionInstance(ue, nrfUri, models.NfType_NSSF,
					models.NfType_AMF, nil); err != nil {
					ue.GmmLog.Errorf("AMF can not select an NSSF Instance by NRF[Error: %+v]", err)
					time.Sleep(2 * time.Second)
				} else {
					break
				}
			}
		}

		response, problemDetails, err := NSSelectionGetForPduSession(ue, snssai)
		if err != nil {
			err = fmt.Errorf("NSSelection Get Error[%+v]", err)
			return nil, nasMessage.Cause5GMMPayloadWasNotForwarded, err
		} else if problemDetails != nil {
			err = fmt.Errorf("NSSelection Get Failed Problem[%+v]", problemDetails)
			return nil, nasMessage.Cause5GMMPayloadWasNotForwarded, err
		}
		nsiInformation = response.NsiInformation
	}

	smContext := amf_context.NewSmContext(pduSessionID)
	smContext.SetSnssai(snssai)
	smContext.SetDnn(dnn)
	smContext.SetAccessType(anType)

	if nsiInformation == nil {
		ue.GmmLog.Warnf("nsiInformation is still nil, use default NRF[%s]", nrfUri)
	} else {
		smContext.SetNsInstance(nsiInformation.NsiId)
		nrfApiUri, err := url.Parse(nsiInformation.NrfId)
		if err != nil {
			ue.GmmLog.Errorf("Parse NRF URI error, use default NRF[%s]", nrfUri)
		} else {
			nrfUri = fmt.Sprintf("%s://%s", nrfApiUri.Scheme, nrfApiUri.Host)
		}
	}

	param := Nnrf_NFDiscovery.SearchNFInstancesParamOpts{
		ServiceNames: optional.NewInterface([]models.ServiceName{models.ServiceName_NSMF_PDUSESSION}),
		Dnn:          optional.NewString(dnn),
		Snssais:      optional.NewInterface(util.MarshToJsonString([]models.Snssai{snssai})),
	}
	if ue.PlmnId.Mcc != "" {
		param.TargetPlmnList = optional.NewInterface(util.MarshToJsonString(ue.PlmnId))
	}

	ue.GmmLog.Debugf("Search SMF from NRF[%s]", nrfUri)
	if ue.SmfUri == "" {
		result, err := SendSearchNFInstances(nrfUri, models.NfType_SMF, models.NfType_AMF, &param)
		if err != nil {
			return nil, nasMessage.Cause5GMMPayloadWasNotForwarded, err
		}
		if len(result.NfInstances) == 0 {
			err = fmt.Errorf("DNN[%s] is not supported or not subscribed in the slice[Snssai: %+v]", dnn, snssai)
			return nil, nasMessage.Cause5GMMDNNNotSupportedOrNotSubscribedInTheSlice, err
		}

		// select the first SMF, TODO: select base on other info
		smContext.SmfProfiles = result.NfInstances
		if amf_context.AMF_Self().MinLbSMF {
			// fmt.Println("Selecting from minLbSMF")
			min_load := math.MaxInt32
			nf := models.NfProfile{}
			amf_context.AMF_Self().SmfNfProfileListMutex.Lock()
			if amf_context.AMF_Self().SmfNfProfileList == nil {
				amf_context.AMF_Self().SmfNfProfileList = make(map[string]int)
			}
			for _, nfProfile := range result.NfInstances {
				// fmt.Println("nfProfile Load: ", nfProfile.Load, nfProfile)
				if int(nfProfile.Load) < min_load {
					nf = nfProfile
					min_load = int(nfProfile.Load)
				}
			}
			smfUri = util.SearchNFServiceUri(nf, models.ServiceName_NSMF_PDUSESSION, models.NfServiceStatus_REGISTERED)
			smContext.SetSmfID(nf.NfInstanceId)
			smContext.SetSmfUri(smfUri)
			ue.SmfUri = smfUri
			ue.SmfNfId = nf.NfInstanceId
			logger.ConsumerLog.Error("for Ue: ", ue.Supi, " for targetNfType ", string(models.NfType_SMF), " NF is: ", nf.Ipv4Addresses, " Load Count: ", min_load)
			amf_context.AMF_Self().SmfNfProfileListMutex.Unlock()
		} else {
			nfInstanceIds := make([]string, 0, len(result.NfInstances))
			for _, nfProfile := range result.NfInstances {
				nfInstanceIds = append(nfInstanceIds, nfProfile.NfInstanceId)
			}
			sort.Strings(nfInstanceIds)
			nfInstanceIdIndexMap := make(map[string]int)
			for index, value := range nfInstanceIds {
				nfInstanceIdIndexMap[value] = index
			}

			nfInstanceIndex := 0
			if amf_context.AMF_Self().EnableScaling {
				parts := strings.Split(ue.Supi, "-")
				imsiNumber, err := strconv.Atoi(parts[1])
				if err != nil {
					logger.ConsumerLog.Errorf("strconv.Atoi error: %+v", err)
					return nil, nasMessage.Cause5GMMPayloadWasNotForwarded, err
				}
				nfInstanceIndex = imsiNumber % len(result.NfInstances)
			}
			for _, nfProfile := range result.NfInstances {
				if nfInstanceIndex != nfInstanceIdIndexMap[nfProfile.NfInstanceId] {
					continue
				}
				smfUri = util.SearchNFServiceUri(nfProfile, models.ServiceName_NSMF_PDUSESSION, models.NfServiceStatus_REGISTERED)
				smContext.SetSmfID(nfProfile.NfInstanceId)
				smContext.SetSmfUri(smfUri)
				ue.SmfUri = smfUri
				ue.SmfNfId = nfProfile.NfInstanceId
				logger.ConsumerLog.Warnln("for Ue: ", ue.Supi, " nfInstanceIndex: ", nfInstanceIndex, " for targetNfType ", string(models.NfType_SMF), " NF is: ", nfProfile.Ipv4Addresses)
				break
			}
		}
	} else {
		smContext.SetSmfID(ue.SmfNfId)
		smContext.SetSmfUri(ue.SmfUri)
	}

	return smContext, 0, nil
}

func SendCreateSmContextRequest(ue *amf_context.AmfUe, smContext *amf_context.SmContext,
	requestType *models.RequestType, nasPdu []byte) (
	response *models.PostSmContextsResponse, smContextRef string, errorResponse *models.PostSmContextsErrorResponse,
	problemDetail *models.ProblemDetails, err1 error,
) {
	smContextCreateData := buildCreateSmContextRequest(ue, smContext, nil)

	postSmContextsRequest := models.PostSmContextsRequest{
		JsonData:              &smContextCreateData,
		BinaryDataN1SmMessage: nasPdu,
	}

	configuration := Nsmf_PDUSession.NewConfiguration()
	configuration.SetBasePath(smContext.SmfUri())
	client := Nsmf_PDUSession.NewAPIClient(configuration)

	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	postSmContextReponse, httpResponse, err := client.SMContextsCollectionApi.PostSmContexts(ctx, postSmContextsRequest)

	if err == nil {
		response = &postSmContextReponse
		smContextRef = httpResponse.Header.Get("Location")
	} else if httpResponse != nil {
		if httpResponse.Status != err.Error() {
			err1 = err
			return response, smContextRef, errorResponse, problemDetail, err1
		}
		switch httpResponse.StatusCode {
		case 400, 403, 404, 500, 503, 504:
			errResponse := err.(openapi.GenericOpenAPIError).Model().(models.PostSmContextsErrorResponse)
			errorResponse = &errResponse
		case 411, 413, 415, 429:
			problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
			problemDetail = &problem
		}
	} else {
		err1 = openapi.ReportError("server no response")
	}
	return response, smContextRef, errorResponse, problemDetail, err1
}

func buildCreateSmContextRequest(ue *amf_context.AmfUe, smContext *amf_context.SmContext,
	requestType *models.RequestType,
) (smContextCreateData models.SmContextCreateData) {
	context := amf_context.AMF_Self()
	smContextCreateData.Supi = ue.Supi
	smContextCreateData.UnauthenticatedSupi = ue.UnauthenticatedSupi
	smContextCreateData.Pei = ue.Pei
	smContextCreateData.Gpsi = ue.Gpsi
	smContextCreateData.PduSessionId = smContext.PduSessionID()
	snssai := smContext.Snssai()
	smContextCreateData.SNssai = &snssai
	smContextCreateData.Dnn = smContext.Dnn()
	smContextCreateData.ServingNfId = context.NfId
	smContextCreateData.Guami = &context.ServedGuamiList[0]
	// take seving networking plmn from userlocation.Tai
	if ue.Tai.PlmnId != nil {
		smContextCreateData.ServingNetwork = ue.Tai.PlmnId
	} else {
		ue.GmmLog.Warnf("Tai is not received from Serving Network, Serving Plmn [Mcc %v, Mnc: %v] is taken from Guami List", context.ServedGuamiList[0].PlmnId.Mcc, context.ServedGuamiList[0].PlmnId.Mnc)
		smContextCreateData.ServingNetwork = context.ServedGuamiList[0].PlmnId
	}
	if requestType != nil {
		smContextCreateData.RequestType = *requestType
	}
	smContextCreateData.N1SmMsg = new(models.RefToBinaryData)
	smContextCreateData.N1SmMsg.ContentId = "n1SmMsg"
	smContextCreateData.AnType = smContext.AccessType()
	if ue.RatType != "" {
		smContextCreateData.RatType = ue.RatType
	}
	// TODO: location is used in roaming scenerio
	// if ue.Location != nil {
	// 	smContextCreateData.UeLocation = ue.Location
	// }
	smContextCreateData.UeTimeZone = ue.TimeZone
	smContextCreateData.SmContextStatusUri = context.GetIPv4Uri() + "/namf-callback/v1/smContextStatus/" +
		ue.Guti + "/" + strconv.Itoa(int(smContext.PduSessionID()))

	return smContextCreateData
}

// Upadate SmContext Request
// servingNfId, smContextStatusUri, guami, servingNetwork -> amf change
// anType -> anType change
// ratType -> ratType change
// presenceInLadn -> Service Request , Xn handover, N2 handover and dnn is a ladn
// ueLocation -> the user location has changed or the user plane of the PDU session is deactivated
// upCnxState -> request the activation or the deactivation of the user plane connection of the PDU session
// hoState -> the preparation, execution or cancellation of a handover of the PDU session
// toBeSwitch -> Xn Handover to request to switch the PDU session to a new downlink N3 tunnel endpoint
// failedToBeSwitch -> indicate that the PDU session failed to be setup in the target RAN
// targetId, targetServingNfId(preparation with AMF change) -> N2 handover
// release -> duplicated PDU Session Id in subclause 5.2.2.3.11, slice not available in subclause 5.2.2.3.12
// ngApCause -> e.g. the NGAP cause for requesting to deactivate the user plane connection of the PDU session.
// 5gMmCauseValue -> AMF received a 5GMM cause code from the UE e.g 5GMM Status message in response to
// a Downlink NAS Transport message carrying 5GSM payload
// anTypeCanBeChanged

func SendUpdateSmContextActivateUpCnxState(
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, accessType models.AccessType) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.UpCnxState = models.UpCnxState_ACTIVATING
	if !amf_context.CompareUserLocation(ue.Location, smContext.UserLocation()) {
		updateData.UeLocation = &ue.Location
	}
	if smContext.AccessType() != accessType {
		updateData.AnType = smContext.AccessType()
	}
	if ladn, ok := ue.ServingAMF.LadnPool[smContext.Dnn()]; ok {
		if amf_context.InTaiList(ue.Tai, ladn.TaiLists) {
			updateData.PresenceInLadn = models.PresenceState_IN_AREA
		}
	}
	return SendUpdateSmContextRequest(smContext, updateData, nil, nil)
}

func SendUpdateSmContextDeactivateUpCnxState(ue *amf_context.AmfUe,
	smContext *amf_context.SmContext, cause amf_context.CauseAll) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.UpCnxState = models.UpCnxState_DEACTIVATED
	updateData.UeLocation = &ue.Location
	if cause.Cause != nil {
		updateData.Cause = *cause.Cause
	}
	if cause.NgapCause != nil {
		updateData.NgApCause = cause.NgapCause
	}
	if cause.Var5GmmCause != nil {
		updateData.Var5gMmCauseValue = *cause.Var5GmmCause
	}
	return SendUpdateSmContextRequest(smContext, updateData, nil, nil)
}

func SendUpdateSmContextChangeAccessType(ue *amf_context.AmfUe,
	smContext *amf_context.SmContext, anTypeCanBeChanged bool) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.AnTypeCanBeChanged = anTypeCanBeChanged
	return SendUpdateSmContextRequest(smContext, updateData, nil, nil)
}

func SendUpdateSmContextN2Info(
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, n2SmType models.N2SmInfoType, N2SmInfo []byte) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.N2SmInfoType = n2SmType
	updateData.N2SmInfo = new(models.RefToBinaryData)
	updateData.N2SmInfo.ContentId = N2SMINFO_ID
	updateData.UeLocation = &ue.Location
	return SendUpdateSmContextRequest(smContext, updateData, nil, N2SmInfo)
}

func SendUpdateSmContextXnHandover(
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, n2SmType models.N2SmInfoType, N2SmInfo []byte) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	// Check if the smContext is nil to prevent nil pointer dereference
	if smContext == nil {
		return nil, nil, nil, fmt.Errorf("smContext is nil")
	}
	updateData := models.SmContextUpdateData{}
	if n2SmType != "" {
		updateData.N2SmInfoType = n2SmType
		updateData.N2SmInfo = new(models.RefToBinaryData)
		updateData.N2SmInfo.ContentId = N2SMINFO_ID
	}
	updateData.ToBeSwitched = true
	updateData.UeLocation = &ue.Location
	if ladn, ok := ue.ServingAMF.LadnPool[smContext.Dnn()]; ok {
		if amf_context.InTaiList(ue.Tai, ladn.TaiLists) {
			updateData.PresenceInLadn = models.PresenceState_IN_AREA
		} else {
			updateData.PresenceInLadn = models.PresenceState_OUT_OF_AREA
		}
	}
	return SendUpdateSmContextRequest(smContext, updateData, nil, N2SmInfo)
}

func SendUpdateSmContextXnHandoverFailed(
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, n2SmType models.N2SmInfoType, N2SmInfo []byte) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	if n2SmType != "" {
		updateData.N2SmInfoType = n2SmType
		updateData.N2SmInfo = new(models.RefToBinaryData)
		updateData.N2SmInfo.ContentId = N2SMINFO_ID
	}
	updateData.FailedToBeSwitched = true
	return SendUpdateSmContextRequest(smContext, updateData, nil, N2SmInfo)
}

func SendUpdateSmContextN2HandoverPreparing(
	ue *amf_context.AmfUe,
	smContext *amf_context.SmContext,
	n2SmType models.N2SmInfoType,
	N2SmInfo []byte, amfid string, targetId *models.NgRanTargetId) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	if n2SmType != "" {
		updateData.N2SmInfoType = n2SmType
		updateData.N2SmInfo = new(models.RefToBinaryData)
		updateData.N2SmInfo.ContentId = N2SMINFO_ID
	}
	updateData.HoState = models.HoState_PREPARING
	updateData.TargetId = targetId
	// amf changed in same plmn
	if amfid != "" {
		updateData.TargetServingNfId = amfid
	}
	return SendUpdateSmContextRequest(smContext, updateData, nil, N2SmInfo)
}

func SendUpdateSmContextN2HandoverPrepared(
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, n2SmType models.N2SmInfoType, N2SmInfo []byte) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	if n2SmType != "" {
		updateData.N2SmInfoType = n2SmType
		updateData.N2SmInfo = new(models.RefToBinaryData)
		updateData.N2SmInfo.ContentId = N2SMINFO_ID
	}
	updateData.HoState = models.HoState_PREPARED
	return SendUpdateSmContextRequest(smContext, updateData, nil, N2SmInfo)
}

func SendUpdateSmContextN2HandoverComplete(
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, amfid string, guami *models.Guami) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.HoState = models.HoState_COMPLETED
	if amfid != "" {
		updateData.ServingNfId = amfid
		updateData.ServingNetwork = guami.PlmnId
		updateData.Guami = guami
	}
	if ladn, ok := ue.ServingAMF.LadnPool[smContext.Dnn()]; ok {
		if amf_context.InTaiList(ue.Tai, ladn.TaiLists) {
			updateData.PresenceInLadn = models.PresenceState_IN_AREA
		} else {
			updateData.PresenceInLadn = models.PresenceState_OUT_OF_AREA
		}
	}
	return SendUpdateSmContextRequest(smContext, updateData, nil, nil)
}

func SendUpdateSmContextN2HandoverCanceled(ue *amf_context.AmfUe,
	smContext *amf_context.SmContext, cause amf_context.CauseAll) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.HoState = models.HoState_CANCELLED
	if cause.Cause != nil {
		updateData.Cause = *cause.Cause
	}
	if cause.NgapCause != nil {
		updateData.NgApCause = cause.NgapCause
	}
	if cause.Var5GmmCause != nil {
		updateData.Var5gMmCauseValue = *cause.Var5GmmCause
	}
	return SendUpdateSmContextRequest(smContext, updateData, nil, nil)
}

func SendUpdateSmContextHandoverBetweenAccessType(
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, targetAccessType models.AccessType, N1SmMsg []byte) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.AnType = targetAccessType
	if N1SmMsg != nil {
		updateData.N1SmMsg = new(models.RefToBinaryData)
		updateData.N1SmMsg.ContentId = "N1Msg"
	}
	return SendUpdateSmContextRequest(smContext, updateData, N1SmMsg, nil)
}

func SendUpdateSmContextHandoverBetweenAMF(
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, amfid string, guami *models.Guami, activate bool) (
	*models.UpdateSmContextResponse, *models.UpdateSmContextErrorResponse, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.ServingNfId = amfid
	updateData.ServingNetwork = guami.PlmnId
	updateData.Guami = guami
	if activate {
		updateData.UpCnxState = models.UpCnxState_ACTIVATING
		if !amf_context.CompareUserLocation(ue.Location, smContext.UserLocation()) {
			updateData.UeLocation = &ue.Location
		}
		if ladn, ok := ue.ServingAMF.LadnPool[smContext.Dnn()]; ok {
			if amf_context.InTaiList(ue.Tai, ladn.TaiLists) {
				updateData.PresenceInLadn = models.PresenceState_IN_AREA
			}
		}
	}
	return SendUpdateSmContextRequest(smContext, updateData, nil, nil)
}

func SendUpdateSmContextRequest(smContext *amf_context.SmContext,
	updateData models.SmContextUpdateData, n1Msg []byte, n2Info []byte) (
	response *models.UpdateSmContextResponse, errorResponse *models.UpdateSmContextErrorResponse,
	problemDetail *models.ProblemDetails, err1 error,
) {
	configuration := Nsmf_PDUSession.NewConfiguration()
	configuration.SetBasePath(smContext.SmfUri())
	client := Nsmf_PDUSession.NewAPIClient(configuration)

	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	// defer cancel()

	var updateSmContextRequest models.UpdateSmContextRequest
	updateSmContextRequest.JsonData = &updateData
	updateSmContextRequest.BinaryDataN1SmMessage = n1Msg
	updateSmContextRequest.BinaryDataN2SmInformation = n2Info

	updateSmContextReponse, httpResponse, err := client.IndividualSMContextApi.UpdateSmContext(ctx, smContext.SmContextRef(),
		updateSmContextRequest)
	//retry on alternate SMF
	retry := 10
	for retry > 0 {
		if err != nil || httpResponse == nil || httpResponse.StatusCode != 200 {
			cancel()
			retry--
			if errProfile := setAltSmfProfile(smContext); errProfile == nil {
				configuration := Nsmf_PDUSession.NewConfiguration()
				fmt.Println("smContext.SmfUri():", smContext.SmfUri(), "retry: ", retry)
				configuration.SetBasePath(smContext.SmfUri())
				client := Nsmf_PDUSession.NewAPIClient(configuration)
				ctx, cancel = context.WithTimeout(context.TODO(), 30*time.Second)

				updateSmContextReponse, httpResponse, err =
					client.IndividualSMContextApi.UpdateSmContext(ctx, smContext.SmContextRef(),
						updateSmContextRequest)
				fmt.Println("Get Alternate SMF 2 after Request: httpResponse", httpResponse, " err: ", err, " updateSmContextReponse: ", updateSmContextReponse)
			}
			time.Sleep(3 * time.Second)
		} else {
			break
		}
	}
	cancel()

	if err == nil {
		response = &updateSmContextReponse
	} else if httpResponse != nil {
		if httpResponse.Status != err.Error() {
			err1 = err
			return response, errorResponse, problemDetail, err1
		}
		switch httpResponse.StatusCode {
		case 400, 403, 404, 500, 503:
			errResponse := err.(openapi.GenericOpenAPIError).Model().(models.UpdateSmContextErrorResponse)
			errorResponse = &errResponse
		case 411, 413, 415, 429:
			problem := err.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
			problemDetail = &problem
		}
	} else {
		err1 = openapi.ReportError("server no response")
	}
	return response, errorResponse, problemDetail, err1
}

// Release SmContext Request

func SendReleaseSmContextRequest(ue *amf_context.AmfUe, smContext *amf_context.SmContext,
	cause *amf_context.CauseAll, n2SmInfoType models.N2SmInfoType,
	n2Info []byte,
) (detail *models.ProblemDetails, err error) {
	configuration := Nsmf_PDUSession.NewConfiguration()
	configuration.SetBasePath(smContext.SmfUri())
	client := Nsmf_PDUSession.NewAPIClient(configuration)

	releaseData := buildReleaseSmContextRequest(ue, cause, n2SmInfoType, n2Info)
	releaseSmContextRequest := models.ReleaseSmContextRequest{
		JsonData: &releaseData,
	}

	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	response, err1 := client.IndividualSMContextApi.ReleaseSmContext(
		ctx, smContext.SmContextRef(), releaseSmContextRequest)

	if err1 == nil {
		ue.SmContextList.Delete(smContext.PduSessionID())
	} else if response != nil && response.Status == err1.Error() {
		problem := err1.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		detail = &problem
	} else {
		err = err1
	}
	return
}

func buildReleaseSmContextRequest(
	ue *amf_context.AmfUe, cause *amf_context.CauseAll, n2SmInfoType models.N2SmInfoType, n2Info []byte) (
	releaseData models.SmContextReleaseData,
) {
	if cause != nil {
		if cause.Cause != nil {
			releaseData.Cause = *cause.Cause
		}
		if cause.NgapCause != nil {
			releaseData.NgApCause = cause.NgapCause
		}
		if cause.Var5GmmCause != nil {
			releaseData.Var5gMmCauseValue = *cause.Var5GmmCause
		}
	}
	if ue.TimeZone != "" {
		releaseData.UeTimeZone = ue.TimeZone
	}
	if n2Info != nil {
		releaseData.N2SmInfoType = n2SmInfoType
		releaseData.N2SmInfo = &models.RefToBinaryData{
			ContentId: N2SMINFO_ID,
		}
	}
	// TODO: other param(ueLocation...)
	return
}
