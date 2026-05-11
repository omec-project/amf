// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/util"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/v2/Nsmf_PDUSession"
	"github.com/omec-project/openapi/v2/models"
	"go.opentelemetry.io/otel/attribute"
)

const N2SMINFO_ID = "N2SmInfo"

func getServingSmfIndex(smfNum int) (servingSmfIndex int) {
	servingSmfIndexStr := os.Getenv("SERVING_SMF_INDEX")
	i := -1
	if servingSmfIndexStr != "" {
		parsedIndex, err := strconv.Atoi(servingSmfIndexStr)
		if err != nil {
			logger.ConsumerLog.Errorf("Could not convert %s to int: %v", servingSmfIndexStr, err)
		} else {
			i = parsedIndex
		}
	}
	if i == -1 {
		i = smfNum - 1
	}
	servingSmfIndexInt := i + 1
	servingSmfIndex = servingSmfIndexInt % smfNum
	if err := os.Setenv("SERVING_SMF_INDEX", strconv.Itoa(servingSmfIndex)); err != nil {
		logger.ConsumerLog.Errorf("Could not set env SERVING_SMF_INDEX: %v", err)
	}
	return
}

func setAltSmfProfile(smCtxt *amf_context.SmContext) error {
	ignoreSmfId := smCtxt.SmfID()
	var altSmfInst []models.NFProfileDiscovery
	// iterate over nf instances to ignore failed NF
	for _, inst := range smCtxt.SmfProfiles {
		if inst.NfInstanceId != ignoreSmfId {
			altSmfInst = append(altSmfInst, inst)
		}
	}

	if len(altSmfInst) > 0 {
		smCtxt.SmfProfiles = altSmfInst
		nfProfile := altSmfInst[0]
		smfUri := util.SearchNFServiceUri(nfProfile, models.SERVICENAME_NSMF_PDUSESSION, models.NFSERVICESTATUS_REGISTERED)
		smCtxt.SetSmfID(nfProfile.NfInstanceId)
		smCtxt.SetSmfUri(smfUri)
		return nil
	}
	return fmt.Errorf("no alternate profiles available")
}

func SelectSmf(
	ctx context.Context,
	ue *amf_context.AmfUe,
	anType models.AccessType,
	pduSessionID int32,
	snssai models.Snssai,
	dnn string,
) (*amf_context.SmContext, uint8, error) {
	var smfUri string

	ue.GmmLog.Infof("Select SMF [snssai: %+v, dnn: %s]", snssai, dnn)
	if snssai.Sst == 0 || dnn == "" {
		return nil, nasMessage.Cause5GMMPayloadWasNotForwarded, fmt.Errorf("invalid SNSSAI or DNN parameters")
	}

	nrfUri := ue.ServingAMF.NrfUri // default NRF URI is pre-configured by AMF

	nsiInformation := ue.GetNsiInformationFromSnssai(anType, snssai)
	if nsiInformation == nil {
		const maxRetries = 10
		for i := range maxRetries {
			if err := SearchNssfNSSelectionInstance(ctx, ue, nrfUri, models.NFTYPE_NSSF, models.NFTYPE_AMF, nil); err != nil {
				ue.GmmLog.Errorf("AMF cannot select an NSSF instance via NRF [error: %+v]", err)
				if i == maxRetries-1 {
					return nil, nasMessage.Cause5GMMPayloadWasNotForwarded,
						fmt.Errorf("NSSF selection instance timed out")
				}
				time.Sleep(2 * time.Second)
				continue
			}
			break
		}
		response, problemDetails, err := NSSelectionGetForPduSession(ctx, ue, snssai)
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
		smContext.SetNsInstance(nsiInformation.GetNsiId())
		nrfApiUri, err := url.Parse(nsiInformation.NrfId)
		if err != nil {
			ue.GmmLog.Errorf("Parse NRF URI error, use default NRF[%s]", nrfUri)
		} else {
			nrfUri = fmt.Sprintf("%s://%s", nrfApiUri.Scheme, nrfApiUri.Host)
		}
	}

	configureSearchSMFRequest := func(request Nnrf_NFDiscovery.ApiSearchNFInstancesRequest) Nnrf_NFDiscovery.ApiSearchNFInstancesRequest {
		request = request.ServiceNames([]models.ServiceName{models.SERVICENAME_NSMF_PDUSESSION})
		request = request.Dnn(dnn)
		request = request.Snssais([]models.Snssai{snssai})
		if ue.PlmnId.Mcc != "" {
			request = request.TargetPlmnList([]models.PlmnId{ue.PlmnId})
		}
		return request
	}

	ue.GmmLog.Debugf("Search SMF from NRF[%s]", nrfUri)

	result, err := SendSearchNFInstances(ctx, nrfUri, models.NFTYPE_SMF, models.NFTYPE_AMF, configureSearchSMFRequest)
	if err != nil {
		return nil, nasMessage.Cause5GMMPayloadWasNotForwarded, err
	}
	if len(result.NfInstances) == 0 && amf_context.AMF_Self().EnableNrfCaching {
		ue.GmmLog.Warnf("SMF discovery via NRF cache returned no instances, retrying direct NRF query")
		directResult, directErr := SendNfDiscoveryToNrf(ctx, nrfUri, models.NFTYPE_SMF, models.NFTYPE_AMF, configureSearchSMFRequest)
		if directErr != nil {
			ue.GmmLog.Errorf("Direct SMF discovery retry failed: %+v", directErr)
		} else if directResult != nil {
			result = directResult
		}
	}

	if len(result.NfInstances) == 0 {
		err = fmt.Errorf("DNN[%s] is not supported or not subscribed in the slice[Snssai: %+v]", dnn, snssai)
		return nil, nasMessage.Cause5GMMDNNNotSupportedOrNotSubscribedInTheSlice, err
	}

	// select the first SMF, TODO: select base on other info
	smContext.SmfProfiles = result.NfInstances
	smfNum := len(result.NfInstances)
	servingSmfIndex := getServingSmfIndex(smfNum)
	nfProfile := result.NfInstances[servingSmfIndex]
	smfUri = util.SearchNFServiceUri(nfProfile, models.SERVICENAME_NSMF_PDUSESSION, models.NFSERVICESTATUS_REGISTERED)
	smContext.SetSmfID(nfProfile.NfInstanceId)
	smContext.SetSmfUri(smfUri)
	return smContext, 0, nil
}

func SendCreateSmContextRequest(ctx context.Context, ue *amf_context.AmfUe, smContext *amf_context.SmContext,
	requestType *models.RequestType, nasPdu []byte) (
	response *models.PostSmContexts201Response, smContextRef string, errorResponse *models.PostSmContexts400Response,
	problemDetail *models.ProblemDetails, err1 error,
) {
	ctx, span := tracer.Start(ctx, "HTTP POST smf/sm-contexts")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "smf"),
		attribute.String("net.peer.name", smContext.SmfUri()),
		attribute.String("amf.nf.id", amf_context.AMF_Self().NfId),
		attribute.String("smf.nf.id", smContext.SmfID()),
		attribute.String("smf.uri", smContext.SmfUri()),
		attribute.String("smf.pdu.session.id", strconv.Itoa(int(smContext.PduSessionID()))),
		attribute.String("smf.snssai.sst", strconv.Itoa(int(smContext.Snssai().Sst))),
		attribute.String("smf.snssai.sd", *smContext.Snssai().Sd),
	)

	smContextCreateData := buildCreateSmContextRequest(ue, smContext, nil)

	// Create a temporary file
	tmpFile, err := os.CreateTemp("", "prefix")
	if err != nil {
		logger.ConsumerLog.Errorln(err)
	}
	defer tmpFile.Close()
	if _, err = tmpFile.Write(nasPdu); err != nil {
		logger.ConsumerLog.Errorln(err)
	}
	if _, err = tmpFile.Seek(0, io.SeekStart); err != nil {
		logger.ConsumerLog.Errorln(err)
	}

	configuration := Nsmf_PDUSession.NewConfiguration()
	cfg := &configuration.Servers[0]
	if apiRootVar, exists := cfg.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = smContext.SmfUri()
		cfg.Variables["apiRoot"] = apiRootVar
	}
	client := Nsmf_PDUSession.NewAPIClient(configuration)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	apiPostSmContextsRequest := client.SMContextsCollectionAPI.PostSmContexts(ctx)
	apiPostSmContextsRequest = apiPostSmContextsRequest.JsonData(smContextCreateData)
	apiPostSmContextsRequest = apiPostSmContextsRequest.BinaryDataN1SmMessage(tmpFile)
	postSmContextReponse, httpResponse, err := client.SMContextsCollectionAPI.PostSmContextsExecute(apiPostSmContextsRequest)
	if err != nil && httpResponse != nil && httpResponse.StatusCode < http.StatusMultipleChoices {
		response = models.NewPostSmContexts201Response()
		if decodeErr := decodeSuccessResponseBody(httpResponse, response); decodeErr == nil {
			if response.JsonData == nil && postSmContextReponse != nil {
				response.JsonData = postSmContextReponse
			}
			smContextRef = httpResponse.Header.Get("Location")
			return response, smContextRef, errorResponse, problemDetail, nil
		}
	}

	if err == nil {
		response = models.NewPostSmContexts201Response()
		if postSmContextReponse != nil {
			response.JsonData = postSmContextReponse
		}
		if httpResponse != nil {
			smContextRef = httpResponse.Header.Get("Location")
		}
	} else if httpResponse != nil {
		if httpResponse.Status != err.Error() {
			err1 = err
			return response, smContextRef, errorResponse, problemDetail, err1
		}
		switch httpResponse.StatusCode {
		case 400, 403, 404, 500, 503, 504:
			if errResponse, ok := openapi.ErrorModel[models.PostSmContexts400Response](err); ok {
				errorResponse = &errResponse
			} else {
				err1 = err
			}
		case 411, 413, 415, 429:
			if problem, ok := openapi.ErrorModel[models.ProblemDetails](err); ok {
				problemDetail = &problem
			} else {
				err1 = err
			}
		}
	} else {
		err1 = err
	}
	return response, smContextRef, errorResponse, problemDetail, err1
}

func buildCreateSmContextRequest(ue *amf_context.AmfUe, smContext *amf_context.SmContext,
	requestType *models.RequestType,
) (smContextCreateData models.SmContextCreateData) {
	context := amf_context.AMF_Self()
	smContextCreateData.Supi = openapi.PtrString(ue.Supi)
	smContextCreateData.UnauthenticatedSupi = openapi.PtrBool(ue.UnauthenticatedSupi)
	smContextCreateData.Pei = openapi.PtrString(ue.Pei)
	smContextCreateData.Gpsi = openapi.PtrString(ue.Gpsi)
	smContextCreateData.PduSessionId = openapi.PtrInt32(smContext.PduSessionID())
	snssai := smContext.Snssai()
	smContextCreateData.SNssai = &snssai
	smContextCreateData.Dnn = openapi.PtrString(smContext.Dnn())
	smContextCreateData.ServingNfId = context.NfId
	smContextCreateData.Guami = &context.ServedGuamiList[0]
	// take seving networking plmn from userlocation.Tai
	if ue.Tai.PlmnId.GetMcc() != "" && ue.Tai.PlmnId.GetMnc() != "" {
		smContextCreateData.ServingNetwork.Mcc = ue.Tai.PlmnId.GetMcc()
		smContextCreateData.ServingNetwork.Mnc = ue.Tai.PlmnId.GetMnc()
	} else {
		ue.GmmLog.Warnf("tai is not received from Serving Network, Serving Plmn [Mcc %s, Mnc: %s] is taken from Guami List", context.ServedGuamiList[0].PlmnId.Mcc, context.ServedGuamiList[0].PlmnId.Mnc)
		smContextCreateData.ServingNetwork = context.ServedGuamiList[0].PlmnId
	}
	if requestType != nil {
		smContextCreateData.RequestType = requestType
	}
	smContextCreateData.N1SmMsg = models.NewRefToBinaryData("n1SmMsg")
	smContextCreateData.AnType = smContext.AccessType()
	if ue.RatType != "" {
		smContextCreateData.RatType = ue.RatType.Ptr()
	}
	// TODO: location is used in roaming scenerio
	// if ue.Location != nil {
	// 	smContextCreateData.UeLocation = ue.Location
	// }
	smContextCreateData.UeTimeZone = openapi.PtrString(ue.TimeZone)
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
	ctx context.Context,
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, accessType models.AccessType) (
	*models.UpdateSmContext200Response, *models.UpdateSmContext400Response, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.UpCnxState = models.UPCNXSTATE_ACTIVATING.Ptr()
	if !amf_context.CompareUserLocation(ue.Location, smContext.UserLocation()) {
		updateData.UeLocation = &ue.Location
	}
	if smContext.AccessType() != accessType {
		updateData.AnType = smContext.AccessType().Ptr()
	}
	if ladn, ok := ue.ServingAMF.LadnPool[smContext.Dnn()]; ok {
		if amf_context.InTaiList(ue.Tai, ladn.TaiLists) {
			updateData.PresenceInLadn = models.PRESENCESTATE_IN_AREA.Ptr()
		}
	}
	return SendUpdateSmContextRequest(ctx, smContext, updateData, nil, nil)
}

func SendUpdateSmContextDeactivateUpCnxState(ctx context.Context, ue *amf_context.AmfUe,
	smContext *amf_context.SmContext, cause amf_context.CauseAll) (
	*models.UpdateSmContext200Response, *models.UpdateSmContext400Response, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.UpCnxState = models.UPCNXSTATE_DEACTIVATED.Ptr()
	updateData.UeLocation = &ue.Location
	if cause.Cause != nil {
		updateData.Cause = cause.Cause
	}
	if cause.NgapCause != nil {
		updateData.NgApCause = cause.NgapCause
	}
	if cause.Var5GmmCause != nil {
		updateData.Var5gMmCauseValue = cause.Var5GmmCause
	}
	return SendUpdateSmContextRequest(ctx, smContext, updateData, nil, nil)
}

func SendUpdateSmContextChangeAccessType(ctx context.Context, ue *amf_context.AmfUe,
	smContext *amf_context.SmContext, anTypeCanBeChanged bool) (
	*models.UpdateSmContext200Response, *models.UpdateSmContext400Response, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.AnTypeCanBeChanged = openapi.PtrBool(anTypeCanBeChanged)
	return SendUpdateSmContextRequest(ctx, smContext, updateData, nil, nil)
}

func SendUpdateSmContextN2Info(
	ctx context.Context,
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, n2SmType models.N2SmInfoType, N2SmInfo []byte) (
	*models.UpdateSmContext200Response, *models.UpdateSmContext400Response, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.N2SmInfoType = &n2SmType
	updateData.N2SmInfo = models.NewRefToBinaryData(N2SMINFO_ID)
	updateData.UeLocation = &ue.Location
	return SendUpdateSmContextRequest(ctx, smContext, updateData, nil, N2SmInfo)
}

func SendUpdateSmContextXnHandover(
	ctx context.Context, ue *amf_context.AmfUe, smContext *amf_context.SmContext, n2SmType models.N2SmInfoType, N2SmInfo []byte) (
	*models.UpdateSmContext200Response, *models.UpdateSmContext400Response, *models.ProblemDetails, error,
) {
	// Check if the smContext is nil to prevent nil pointer dereference
	if smContext == nil {
		return nil, nil, nil, fmt.Errorf("smContext is nil")
	}
	updateData := models.SmContextUpdateData{}
	if n2SmType != "" {
		updateData.N2SmInfoType = &n2SmType
		updateData.N2SmInfo = models.NewRefToBinaryData(N2SMINFO_ID)
	}
	updateData.ToBeSwitched = openapi.PtrBool(true)
	updateData.UeLocation = &ue.Location
	if ladn, ok := ue.ServingAMF.LadnPool[smContext.Dnn()]; ok {
		if amf_context.InTaiList(ue.Tai, ladn.TaiLists) {
			updateData.PresenceInLadn = models.PRESENCESTATE_IN_AREA.Ptr()
		} else {
			updateData.PresenceInLadn = models.PRESENCESTATE_OUT_OF_AREA.Ptr()
		}
	}
	return SendUpdateSmContextRequest(ctx, smContext, updateData, nil, N2SmInfo)
}

func SendUpdateSmContextXnHandoverFailed(
	ctx context.Context,
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, n2SmType models.N2SmInfoType, N2SmInfo []byte) (
	*models.UpdateSmContext200Response, *models.UpdateSmContext400Response, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	if n2SmType != "" {
		updateData.N2SmInfoType = &n2SmType
		updateData.N2SmInfo = models.NewRefToBinaryData(N2SMINFO_ID)
	}
	updateData.FailedToBeSwitched = openapi.PtrBool(true)
	return SendUpdateSmContextRequest(ctx, smContext, updateData, nil, N2SmInfo)
}

func SendUpdateSmContextN2HandoverPreparing(
	ctx context.Context,
	ue *amf_context.AmfUe,
	smContext *amf_context.SmContext,
	n2SmType models.N2SmInfoType,
	N2SmInfo []byte, amfid string, targetId *models.NgRanTargetId) (
	*models.UpdateSmContext200Response, *models.UpdateSmContext400Response, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	if n2SmType != "" {
		updateData.N2SmInfoType = &n2SmType
		updateData.N2SmInfo = models.NewRefToBinaryData(N2SMINFO_ID)
	}
	updateData.HoState = models.HOSTATE_PREPARING.Ptr()
	updateData.TargetId = targetId
	// amf changed in same plmn
	if amfid != "" {
		updateData.TargetServingNfId = openapi.PtrString(amfid)
	}
	return SendUpdateSmContextRequest(ctx, smContext, updateData, nil, N2SmInfo)
}

func SendUpdateSmContextN2HandoverPrepared(
	ctx context.Context,
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, n2SmType models.N2SmInfoType, N2SmInfo []byte) (
	*models.UpdateSmContext200Response, *models.UpdateSmContext400Response, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	if n2SmType != "" {
		updateData.N2SmInfoType = n2SmType.Ptr()
		updateData.N2SmInfo = models.NewRefToBinaryData(N2SMINFO_ID)
	}
	updateData.HoState = models.HOSTATE_PREPARED.Ptr()
	return SendUpdateSmContextRequest(ctx, smContext, updateData, nil, N2SmInfo)
}

func SendUpdateSmContextN2HandoverComplete(
	ctx context.Context,
	ue *amf_context.AmfUe, smContext *amf_context.SmContext, amfid string, guami *models.Guami) (
	*models.UpdateSmContext200Response, *models.UpdateSmContext400Response, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.HoState = models.HOSTATE_COMPLETED.Ptr()
	if amfid != "" {
		updateData.ServingNfId = openapi.PtrString(amfid)
		updateData.ServingNetwork = &guami.PlmnId
		updateData.Guami = guami
	}
	if ladn, ok := ue.ServingAMF.LadnPool[smContext.Dnn()]; ok {
		if amf_context.InTaiList(ue.Tai, ladn.TaiLists) {
			updateData.PresenceInLadn = models.PRESENCESTATE_IN_AREA.Ptr()
		} else {
			updateData.PresenceInLadn = models.PRESENCESTATE_OUT_OF_AREA.Ptr()
		}
	}
	return SendUpdateSmContextRequest(ctx, smContext, updateData, nil, nil)
}

func SendUpdateSmContextN2HandoverCanceled(
	ctx context.Context, ue *amf_context.AmfUe, smContext *amf_context.SmContext, cause amf_context.CauseAll) (
	*models.UpdateSmContext200Response, *models.UpdateSmContext400Response, *models.ProblemDetails, error,
) {
	updateData := models.SmContextUpdateData{}
	updateData.HoState = models.HOSTATE_CANCELLED.Ptr()
	if cause.Cause != nil {
		updateData.Cause = cause.Cause
	}
	if cause.NgapCause != nil {
		updateData.NgApCause = cause.NgapCause
	}
	if cause.Var5GmmCause != nil {
		updateData.Var5gMmCauseValue = cause.Var5GmmCause
	}
	return SendUpdateSmContextRequest(ctx, smContext, updateData, nil, nil)
}

func SendUpdateSmContextRequest(ctx context.Context, smContext *amf_context.SmContext,
	updateData models.SmContextUpdateData, n1Msg []byte, n2Info []byte) (
	response *models.UpdateSmContext200Response, errorResponse *models.UpdateSmContext400Response,
	problemDetail *models.ProblemDetails, err1 error,
) {
	ctx, span := tracer.Start(ctx, "HTTP PUT smf/sm-contexts/{smContextRef}")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "PUT"),
		attribute.String("nf.target", "smf"),
		attribute.String("net.peer.name", smContext.SmfUri()),
		attribute.String("amf.nf.id", amf_context.AMF_Self().NfId),
		attribute.String("smf.nf.id", smContext.SmfID()),
		attribute.String("smf.uri", smContext.SmfUri()),
	)

	configuration := Nsmf_PDUSession.NewConfiguration()
	cfg := &configuration.Servers[0]
	if apiRootVar, exists := cfg.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = smContext.SmfUri()
		cfg.Variables["apiRoot"] = apiRootVar
	}
	client := Nsmf_PDUSession.NewAPIClient(configuration)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	tmpN1File, err := createBinaryPayloadTempFile(n1Msg)
	if err != nil {
		return response, errorResponse, problemDetail, err
	}
	if tmpN1File != nil {
		defer os.Remove(tmpN1File.Name())
	}

	tmpN2File, err := createBinaryPayloadTempFile(n2Info)
	if err != nil {
		return response, errorResponse, problemDetail, err
	}
	if tmpN2File != nil {
		defer os.Remove(tmpN2File.Name())
	}

	apiUpdateSmContextRequest := client.IndividualSMContextAPI.UpdateSmContext(ctx, smContext.SmContextRef())
	apiUpdateSmContextRequest = apiUpdateSmContextRequest.SmContextUpdateData(updateData)
	if tmpN1File != nil {
		apiUpdateSmContextRequest = apiUpdateSmContextRequest.BinaryDataN1SmMessage(tmpN1File)
	}
	if tmpN2File != nil {
		apiUpdateSmContextRequest = apiUpdateSmContextRequest.BinaryDataN2SmInformation(tmpN2File)
	}
	updateSmContextReponse, httpResponse, err := client.IndividualSMContextAPI.UpdateSmContextExecute(apiUpdateSmContextRequest)
	// retry on alternate SMF
	if err != nil {
		if errProfile := setAltSmfProfile(smContext); errProfile == nil {
			configuration := Nsmf_PDUSession.NewConfiguration()
			cfg := &configuration.Servers[0]
			if apiRootVar, exists := cfg.Variables["apiRoot"]; exists {
				apiRootVar.DefaultValue = smContext.SmfUri()
				cfg.Variables["apiRoot"] = apiRootVar
			}
			client := Nsmf_PDUSession.NewAPIClient(configuration)

			retryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			retryCtx, span := tracer.Start(retryCtx, "HTTP PUT smf/sm-contexts/{smContextRef}/modify")
			defer span.End()

			span.SetAttributes(
				attribute.String("http.method", "PUT"),
				attribute.String("nf.target", "smf"),
				attribute.String("net.peer.name", smContext.SmfUri()),
			)

			tmpN1File, err = createBinaryPayloadTempFile(n1Msg)
			if err != nil {
				return response, errorResponse, problemDetail, err
			}
			if tmpN1File != nil {
				defer os.Remove(tmpN1File.Name())
			}

			tmpN2File, err = createBinaryPayloadTempFile(n2Info)
			if err != nil {
				return response, errorResponse, problemDetail, err
			}
			if tmpN2File != nil {
				defer os.Remove(tmpN2File.Name())
			}

			apiUpdateSmContextRequest := client.IndividualSMContextAPI.UpdateSmContext(retryCtx, smContext.SmContextRef())
			apiUpdateSmContextRequest = apiUpdateSmContextRequest.SmContextUpdateData(updateData)
			if tmpN1File != nil {
				apiUpdateSmContextRequest = apiUpdateSmContextRequest.BinaryDataN1SmMessage(tmpN1File)
			}
			if tmpN2File != nil {
				apiUpdateSmContextRequest = apiUpdateSmContextRequest.BinaryDataN2SmInformation(tmpN2File)
			}
			updateSmContextReponse, httpResponse, err = client.IndividualSMContextAPI.UpdateSmContextExecute(apiUpdateSmContextRequest)
		}
	}

	if err == nil {
		response = models.NewUpdateSmContext200Response()
		if updateSmContextReponse != nil {
			response.JsonData = updateSmContextReponse
		}
	} else if httpResponse != nil && httpResponse.StatusCode < http.StatusMultipleChoices {
		response = models.NewUpdateSmContext200Response()
		if decodeErr := decodeSuccessResponseBody(httpResponse, response); decodeErr == nil {
			if response.JsonData == nil && updateSmContextReponse != nil {
				response.JsonData = updateSmContextReponse
			}
			return response, errorResponse, problemDetail, nil
		}
	} else if httpResponse != nil {
		if httpResponse.Status != err.Error() {
			err1 = err
			return response, errorResponse, problemDetail, err1
		}
		switch httpResponse.StatusCode {
		case 400, 403, 404, 500, 503:
			if errResponse, ok := openapi.ErrorModel[models.UpdateSmContext400Response](err); ok {
				errorResponse = &errResponse
			} else {
				err1 = err
			}
		case 411, 413, 415, 429:
			if problem, ok := openapi.ErrorModel[models.ProblemDetails](err); ok {
				problemDetail = &problem
			} else {
				err1 = err
			}
		}
	} else {
		err1 = err
	}
	return response, errorResponse, problemDetail, err1
}

func decodeSuccessResponseBody(httpResponse *http.Response, target any) error {
	if httpResponse == nil || httpResponse.Body == nil {
		return fmt.Errorf("success response body is empty")
	}

	body, err := io.ReadAll(httpResponse.Body)
	if err != nil {
		return err
	}
	if err = httpResponse.Body.Close(); err != nil {
		return err
	}
	httpResponse.Body = io.NopCloser(bytes.NewBuffer(body))
	if len(body) == 0 {
		return nil
	}

	return openapi.Decode(target, body, httpResponse.Header.Get("Content-Type"))
}

func createBinaryPayloadTempFile(payload []byte) (*os.File, error) {
	if len(payload) == 0 {
		return nil, nil
	}

	tmpFile, err := os.CreateTemp("", "prefix")
	if err != nil {
		return nil, err
	}

	if _, err = tmpFile.Write(payload); err != nil {
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

// Release SmContext Request

func SendReleaseSmContextRequest(ue *amf_context.AmfUe, smContext *amf_context.SmContext,
	cause *amf_context.CauseAll, n2SmInfoType models.N2SmInfoType,
	n2Info []byte,
) (detail *models.ProblemDetails, err error) {
	configuration := Nsmf_PDUSession.NewConfiguration()
	cfg := &configuration.Servers[0]
	if apiRootVar, exists := cfg.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = smContext.SmfUri()
		cfg.Variables["apiRoot"] = apiRootVar
	}
	client := Nsmf_PDUSession.NewAPIClient(configuration)

	releaseData := buildReleaseSmContextRequest(ue, cause, n2SmInfoType, n2Info)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	ctx, span := tracer.Start(ctx, "HTTP POST smf/sm-contexts/{smContextRef}/release")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "smf"),
		attribute.String("net.peer.name", smContext.SmfUri()),
	)

	apiReleaseSmContextRequest := client.IndividualSMContextAPI.ReleaseSmContext(ctx, smContext.SmContextRef())
	apiReleaseSmContextRequest = apiReleaseSmContextRequest.SmContextReleaseData(releaseData)
	_, response, err1 := client.IndividualSMContextAPI.ReleaseSmContextExecute(apiReleaseSmContextRequest)

	if err1 == nil {
		ue.SmContextList.Delete(smContext.PduSessionID())
	} else if response != nil && response.Status == err1.Error() {
		if problem, ok := openapi.ErrorModel[models.ProblemDetails](err1); ok {
			detail = &problem
		} else {
			err = err1
		}
	} else {
		err = err1
	}
	return detail, err
}

func buildReleaseSmContextRequest(
	ue *amf_context.AmfUe, cause *amf_context.CauseAll, n2SmInfoType models.N2SmInfoType, n2Info []byte) (
	releaseData models.SmContextReleaseData,
) {
	if cause != nil {
		if cause.Cause != nil {
			releaseData.Cause = cause.Cause
			releaseData.NgApCause = cause.NgapCause
		}
		if cause.Var5GmmCause != nil {
			releaseData.Var5gMmCauseValue = cause.Var5GmmCause
		}
	}
	if ue.TimeZone != "" {
		releaseData.UeTimeZone = openapi.PtrString(ue.TimeZone)
	}
	if n2Info != nil {
		releaseData.N2SmInfoType = n2SmInfoType.Ptr()
		releaseData.N2SmInfo = &models.RefToBinaryData{
			ContentId: N2SMINFO_ID,
		}
	}
	// TODO: other param(ueLocation...)
	return
}
