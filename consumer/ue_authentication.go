// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"time"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/nas/v2/nasType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Nausf_UEAuthentication"
	"github.com/omec-project/openapi/v2/models"
	"go.opentelemetry.io/otel/attribute"
)

func servingNetworkPlmnID(ue *amf_context.AmfUe, servedGuami models.Guami) *models.PlmnIdNid {
	if ue.Tai.PlmnId.GetMcc() != "" && ue.Tai.PlmnId.GetMnc() != "" {
		return models.NewPlmnIdNid(ue.Tai.PlmnId.GetMcc(), ue.Tai.PlmnId.GetMnc())
	}

	if ue.GmmLog != nil {
		ue.GmmLog.Warnf(
			"Tai is not received from Serving Network, Serving Plmn [Mcc: %v Mnc: %v] is taken from Guami List",
			servedGuami.PlmnId.Mcc,
			servedGuami.PlmnId.Mnc,
		)
	}

	return &servedGuami.PlmnId
}

func SendUEAuthenticationAuthenticateRequest(ctx context.Context, ue *amf_context.AmfUe,
	resynchronizationInfo *models.ResynchronizationInfo,
) (*models.UEAuthenticationCtx, *models.ProblemDetails, error) {
	configuration := Nausf_UEAuthentication.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.AusfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}

	client := Nausf_UEAuthentication.NewAPIClient(configuration)

	amfSelf := amf_context.AMF_Self()
	servedGuami := amfSelf.ServedGuamiList[0]
	plmnId := servingNetworkPlmnID(ue, servedGuami)

	var authInfo models.AuthenticationInfo
	authInfo.SupiOrSuci = ue.Suci
	if mnc, err := strconv.Atoi(plmnId.Mnc); err != nil {
		return nil, nil, err
	} else {
		authInfo.ServingNetworkName = fmt.Sprintf("5G:mnc%03d.mcc%s.3gppnetwork.org", mnc, plmnId.Mcc)
	}
	if resynchronizationInfo != nil {
		authInfo.ResynchronizationInfo = resynchronizationInfo
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ctx, span := tracer.Start(ctx, "HTTP POST ausf/ue-authentications")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "ausf"),
		attribute.String("net.peer.name", ue.AusfUri),
	)

	apiUeAuthenticationsPostRequest := client.DefaultAPI.UeAuthenticationsPost(ctx)
	apiUeAuthenticationsPostRequest = apiUeAuthenticationsPostRequest.AuthenticationInfo(authInfo)
	ueAuthenticationCtx, httpResponse, err := client.DefaultAPI.UeAuthenticationsPostExecute(apiUeAuthenticationsPostRequest)
	if err == nil {
		return ueAuthenticationCtx, nil, nil
	} else if httpResponse != nil {
		if httpResponse.Status != err.Error() {
			return nil, nil, err
		}
		if problem, ok := openapi.ErrorModel[models.ProblemDetails](err); ok {
			return nil, &problem, nil
		}
		return nil, nil, err
	} else {
		return nil, nil, openapi.ReportError("server no response")
	}
}

func SendAuth5gAkaConfirmRequest(ctx context.Context, ue *amf_context.AmfUe, resStar string) (
	*models.ConfirmationDataResponse, *models.ProblemDetails, error,
) {
	var ausfUri string
	if confirmUri, err := url.Parse(ue.AuthenticationCtx.Links["link"].Link.GetHref()); err != nil {
		return nil, nil, err
	} else {
		ausfUri = fmt.Sprintf("%s://%s", confirmUri.Scheme, confirmUri.Host)
	}

	configuration := Nausf_UEAuthentication.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ausfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nausf_UEAuthentication.NewAPIClient(configuration)

	confirmData := models.NewConfirmationData(*openapi.NewNullableString(&resStar))

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ctx, span := tracer.Start(ctx, "HTTP PUT ausf/ue-authentications/{authCtxId}/5g-aka-confirmation")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "PUT"),
		attribute.String("nf.target", "ausf"),
		attribute.String("net.peer.name", ausfUri),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	apiUeAuthenticationsAuthCtxId5gAkaConfirmationPutRequest := client.DefaultAPI.UeAuthenticationsAuthCtxId5gAkaConfirmationPut(
		ctx, ue.Suci)
	apiUeAuthenticationsAuthCtxId5gAkaConfirmationPutRequest = apiUeAuthenticationsAuthCtxId5gAkaConfirmationPutRequest.ConfirmationData(*confirmData)
	confirmResult, httpResponse, err := client.DefaultAPI.UeAuthenticationsAuthCtxId5gAkaConfirmationPutExecute(apiUeAuthenticationsAuthCtxId5gAkaConfirmationPutRequest)
	if err == nil {
		return confirmResult, nil, nil
	} else if httpResponse != nil {
		if httpResponse.Status != err.Error() {
			return nil, nil, err
		}
		switch httpResponse.StatusCode {
		case 400, 500:
			if problem, ok := openapi.ErrorModel[models.ProblemDetails](err); ok {
				return nil, &problem, nil
			}
			return nil, nil, err
		}
		return nil, nil, nil
	} else {
		return nil, nil, openapi.ReportError("server no response")
	}
}

func SendEapAuthConfirmRequest(ctx context.Context, ue *amf_context.AmfUe, eapMsg nasType.EAPMessage) (
	response *models.EapSession, problemDetails *models.ProblemDetails, err1 error,
) {
	confirmUri, err := url.Parse(ue.AuthenticationCtx.Links["link"].Link.GetHref())
	if err != nil {
		logger.ConsumerLog.Errorf("url Parse failed: %+v", err)
	}
	ausfUri := fmt.Sprintf("%s://%s", confirmUri.Scheme, confirmUri.Host)

	configuration := Nausf_UEAuthentication.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ausfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nausf_UEAuthentication.NewAPIClient(configuration)

	eapPayload := base64.StdEncoding.EncodeToString(eapMsg.GetEAPMessage())
	eapSession := models.NewEapSession(*openapi.NewNullableString(&eapPayload))
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ctx, span := tracer.Start(ctx, "HTTP POST ausf/ue-authentications/{authCtxId}/eap-session")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "ausf"),
		attribute.String("net.peer.name", ausfUri),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	apiEapAuthMethodRequest := client.DefaultAPI.EapAuthMethod(ctx, ue.Suci)
	apiEapAuthMethodRequest = apiEapAuthMethodRequest.EapSession(*eapSession)
	eapSessionRsp, httpResponse, err := client.DefaultAPI.EapAuthMethodExecute(apiEapAuthMethodRequest)
	if err == nil {
		response = eapSessionRsp
	} else if httpResponse != nil {
		if httpResponse.Status != err.Error() {
			err1 = err
			return response, problemDetails, err1
		}
		switch httpResponse.StatusCode {
		case 400, 500:
			if problem, ok := openapi.ErrorModel[models.ProblemDetails](err); ok {
				problemDetails = &problem
			} else {
				err1 = err
			}
		}
	} else {
		err1 = openapi.ReportError("server no response")
	}

	return response, problemDetails, err1
}
