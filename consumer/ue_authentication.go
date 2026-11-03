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

	amfContext "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/nas/v2/nasType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Nausf_UEAuthentication"
	"github.com/omec-project/openapi/v2/models"
	"go.opentelemetry.io/otel/attribute"
)

func servingNetworkPlmnID(ue *amfContext.AmfUe, servedGuami models.Guami) *models.PlmnIdNid {
	if ue.Tai.PlmnId.GetMcc() != "" && ue.Tai.PlmnId.GetMnc() != "" {
		return models.NewPlmnIdNid(ue.Tai.PlmnId.GetMcc(), ue.Tai.PlmnId.GetMnc())
	}

	ue.GmmLog.Warnf(
		"Tai is not received from Serving Network, Serving Plmn is taken from Guami List: %+v",
		servedGuami.GetPlmnId(),
	)

	plmnId := servedGuami.GetPlmnId()
	return &plmnId
}

func SendUEAuthenticationAuthenticateRequest(ctx context.Context, ue *amfContext.AmfUe,
	resynchronizationInfo *models.ResynchronizationInfo,
) (*models.UEAuthenticationCtx, *models.ProblemDetails, error) {
	configuration := Nausf_UEAuthentication.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.AusfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}

	client := Nausf_UEAuthentication.NewAPIClient(configuration)

	amfSelf := amfContext.AMF_Self()
	servedGuami := amfSelf.ServedGuamiList[0]
	plmnId := servingNetworkPlmnID(ue, servedGuami)

	mnc, err := strconv.Atoi(plmnId.GetMnc())
	if err != nil {
		return nil, nil, err
	}
	authInfo := models.NewAuthenticationInfo(ue.Suci, fmt.Sprintf("5G:mnc%03d.mcc%s.3gppnetwork.org", mnc, plmnId.GetMcc()))
	if resynchronizationInfo != nil {
		authInfo.SetResynchronizationInfo(*resynchronizationInfo)
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
	apiUeAuthenticationsPostRequest = apiUeAuthenticationsPostRequest.AuthenticationInfo(*authInfo)
	ueAuthenticationCtx, httpResponse, err := client.DefaultAPI.UeAuthenticationsPostExecute(apiUeAuthenticationsPostRequest)
	if httpResponse != nil {
		defer func() {
			if closeErr := httpResponse.Body.Close(); closeErr != nil {
				logger.ConsumerLog.Errorf("SendUEAuthenticationAuthenticateRequest response body cannot close: %+v", closeErr)
			}
		}()
	}
	if err == nil {
		return ueAuthenticationCtx, nil, nil
	} else if httpResponse != nil {
		if problem, ok := openapi.ErrorModel[models.ProblemDetails](err); ok {
			return nil, &problem, nil
		}
		return nil, nil, err
	} else {
		return nil, nil, openapi.ReportError("server no response")
	}
}

// _links member names of the AUSF authentication responses (TS 29.509).
const (
	LinkKey5gAka      = "5g-aka"
	LinkKeyEapSession = "eap-session"
	linkKeyLegacy     = "link"
)

// confirmationHref returns the href of the _links member named by TS 29.509 (specKey), falling back to
// the legacy "link" member sent by AUSFs that predate the spec-compliant names. A member is either a
// single link or an array of links (models.LinksValueSchema); the first nonempty href is used. Members
// without a usable href are skipped so that they do not shadow a usable one.
func confirmationHref(links map[string]models.LinksValueSchema, specKey string) string {
	for _, key := range []string{specKey, linkKeyLegacy} {
		v, ok := links[key]
		if !ok {
			continue
		}
		if v.Link != nil && v.Link.GetHref() != "" {
			return v.Link.GetHref()
		}
		if v.ArrayOfLink != nil {
			for _, l := range *v.ArrayOfLink {
				if l.GetHref() != "" {
					return l.GetHref()
				}
			}
		}
	}
	return ""
}

// HasConfirmationHref reports whether links holds a usable confirmation href for specKey or the legacy name.
func HasConfirmationHref(links map[string]models.LinksValueSchema, specKey string) bool {
	return confirmationHref(links, specKey) != ""
}

// ausfRootUri returns the scheme and host of the AUSF confirmation href for specKey.
func ausfRootUri(links map[string]models.LinksValueSchema, specKey string) (string, error) {
	href := confirmationHref(links, specKey)
	if href == "" {
		return "", fmt.Errorf("no AUSF confirmation link in authentication context")
	}
	confirmUri, err := url.Parse(href)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s://%s", confirmUri.Scheme, confirmUri.Host), nil
}

func SendAuth5gAkaConfirmRequest(ctx context.Context, ue *amfContext.AmfUe, resStar string) (
	*models.ConfirmationDataResponse, *models.ProblemDetails, error,
) {
	ausfUri, err := ausfRootUri(ue.AuthenticationCtx.Links, LinkKey5gAka)
	if err != nil {
		return nil, nil, err
	}

	configuration := Nausf_UEAuthentication.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ausfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nausf_UEAuthentication.NewAPIClient(configuration)

	confirmData := models.NewConfirmationDataWithDefaults()
	confirmData.SetResStar(resStar)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ctx, span := tracer.Start(ctx, "HTTP PUT ausf/ue-authentications/{authCtxId}/5g-aka-confirmation")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "PUT"),
		attribute.String("nf.target", "ausf"),
		attribute.String("net.peer.name", ausfUri),
		attribute.String("plmn.id", ue.PlmnId.GetMcc()+ue.PlmnId.GetMnc()),
	)

	apiUeAuthenticationsAuthCtxId5gAkaConfirmationPutRequest := client.DefaultAPI.UeAuthenticationsAuthCtxId5gAkaConfirmationPut(
		ctx, ue.Suci)
	apiUeAuthenticationsAuthCtxId5gAkaConfirmationPutRequest = apiUeAuthenticationsAuthCtxId5gAkaConfirmationPutRequest.ConfirmationData(*confirmData)
	confirmResult, httpResponse, err := client.DefaultAPI.UeAuthenticationsAuthCtxId5gAkaConfirmationPutExecute(apiUeAuthenticationsAuthCtxId5gAkaConfirmationPutRequest)
	if httpResponse != nil {
		defer func() {
			if closeErr := httpResponse.Body.Close(); closeErr != nil {
				logger.ConsumerLog.Errorf("SendAuth5gAkaConfirmRequest response body cannot close: %+v", closeErr)
			}
		}()
	}
	if err == nil {
		return confirmResult, nil, nil
	} else if httpResponse != nil {
		switch httpResponse.StatusCode {
		case 400, 500:
			if problem, ok := openapi.ErrorModel[models.ProblemDetails](err); ok {
				return nil, &problem, nil
			}
			return nil, nil, err
		}
		// A status this switch does not name still failed, so it is reported rather than returned
		// as a success with nothing in it.
		return nil, nil, err
	} else {
		return nil, nil, openapi.ReportError("server no response")
	}
}

func SendEapAuthConfirmRequest(ctx context.Context, ue *amfContext.AmfUe, eapMsg nasType.EAPMessage) (
	response *models.EapSession, problemDetails *models.ProblemDetails, err1 error,
) {
	ausfUri, err := ausfRootUri(ue.AuthenticationCtx.Links, LinkKeyEapSession)
	if err != nil {
		logger.ConsumerLog.Errorf("AUSF confirmation URI failed: %+v", err)
		return nil, nil, err
	}

	configuration := Nausf_UEAuthentication.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ausfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nausf_UEAuthentication.NewAPIClient(configuration)

	eapPayload := base64.StdEncoding.EncodeToString(eapMsg.GetEAPMessage())
	eapSession := models.NewEapSessionWithDefaults()
	eapSession.SetEapPayload(eapPayload)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	ctx, span := tracer.Start(ctx, "HTTP POST ausf/ue-authentications/{authCtxId}/eap-session")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "ausf"),
		attribute.String("net.peer.name", ausfUri),
		attribute.String("plmn.id", ue.PlmnId.GetMcc()+ue.PlmnId.GetMnc()),
	)

	apiEapAuthMethodRequest := client.DefaultAPI.EapAuthMethod(ctx, ue.Suci)
	apiEapAuthMethodRequest = apiEapAuthMethodRequest.EapSession(*eapSession)
	eapSessionRsp, httpResponse, err := client.DefaultAPI.EapAuthMethodExecute(apiEapAuthMethodRequest)
	if httpResponse != nil {
		defer func() {
			if closeErr := httpResponse.Body.Close(); closeErr != nil {
				logger.ConsumerLog.Errorf("SendEapAuthConfirmRequest response body cannot close: %+v", closeErr)
			}
		}()
	}
	if err == nil {
		response = eapSessionRsp
	} else if httpResponse != nil {
		switch httpResponse.StatusCode {
		case 400, 500:
			if problem, ok := openapi.ErrorModel[models.ProblemDetails](err); ok {
				problemDetails = &problem
			} else {
				err1 = err
			}
		default:
			err1 = err
		}
	} else {
		err1 = openapi.ReportError("server no response")
	}

	return response, problemDetails, err1
}
