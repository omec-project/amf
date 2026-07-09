// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"time"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Nudm_UECM"
	"github.com/omec-project/openapi/v2/models"
	"go.opentelemetry.io/otel/attribute"
)

func ensureRatType(ue *amf_context.AmfUe, accessType models.AccessType) models.RatType {
	if ue.RatType != "" {
		return ue.RatType
	}

	switch accessType {
	case models.ACCESSTYPE__3_GPP_ACCESS:
		ue.RatType = models.RATTYPE_NR
	case models.ACCESSTYPE_NON_3_GPP_ACCESS:
		ue.RatType = models.RATTYPE_WLAN
	}

	return ue.RatType
}

func UeCmRegistration(ctx context.Context, ue *amf_context.AmfUe, accessType models.AccessType, initialRegistrationInd bool) (
	*models.ProblemDetails, error,
) {
	configuration := Nudm_UECM.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.NudmUECMUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nudm_UECM.NewAPIClient(configuration)

	amfSelf := amf_context.AMF_Self()

	switch accessType {
	case models.ACCESSTYPE__3_GPP_ACCESS:
		ratType := ensureRatType(ue, accessType)
		registrationData := models.Amf3GppAccessRegistration{
			AmfInstanceId:          amfSelf.NfId,
			InitialRegistrationInd: openapi.PtrBool(initialRegistrationInd),
			Guami:                  amfSelf.ServedGuamiList[0],
			RatType:                ratType,
			ImsVoPs:                models.IMSVOPS_HOMOGENEOUS_NON_SUPPORT.Ptr(),
		}
		gppAccessCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		gppAccessCtx, span := tracer.Start(gppAccessCtx, "HTTP PUT udm/{ueId}/registrations/amf-3gpp-access")
		defer span.End()

		span.SetAttributes(
			attribute.String("http.method", "PUT"),
			attribute.String("nf.target", "udm"),
			attribute.String("net.peer.name", ue.NudmUECMUri),
			attribute.String("udm.supi", ue.GetSupi()),
			attribute.String("plmn.id", ue.PlmnId.GetMcc()+ue.PlmnId.GetMnc()),
		)

		apiCall3GppRegistrationRequest := client.AMFRegistrationFor3GPPAccessAPI.Call3GppRegistration(gppAccessCtx, ue.GetSupi())
		apiCall3GppRegistrationRequest = apiCall3GppRegistrationRequest.Amf3GppAccessRegistration(registrationData)
		_, httpResp, localErr := client.AMFRegistrationFor3GPPAccessAPI.Call3GppRegistrationExecute(apiCall3GppRegistrationRequest)
		if localErr == nil {
			return nil, nil
		} else if httpResp != nil {
			if httpResp.Status != localErr.Error() {
				return nil, localErr
			}
			if problem, ok := openapi.ErrorModel[models.ProblemDetails](localErr); ok {
				return &problem, nil
			}
			return nil, localErr
		} else {
			return nil, openapi.ReportError("server no response")
		}

	case models.ACCESSTYPE_NON_3_GPP_ACCESS:
		ratType := ensureRatType(ue, accessType)
		registrationData := models.AmfNon3GppAccessRegistration{
			AmfInstanceId: amfSelf.NfId,
			Guami:         amfSelf.ServedGuamiList[0],
			RatType:       ratType,
		}

		non3gppAccessCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		non3gppAccessCtx, span := tracer.Start(non3gppAccessCtx, "HTTP PUT udm/{ueId}/registrations/amf-non-3gpp-access")
		defer span.End()

		span.SetAttributes(
			attribute.String("http.method", "PUT"),
			attribute.String("nf.target", "udm"),
			attribute.String("net.peer.name", ue.NudmUECMUri),
			attribute.String("udm.supi", ue.GetSupi()),
			attribute.String("plmn.id", ue.PlmnId.GetMcc()+ue.PlmnId.GetMnc()),
		)

		apiNon3GppRegistrationRequest := client.AMFRegistrationForNon3GPPAccessAPI.Non3GppRegistration(non3gppAccessCtx, ue.GetSupi())
		apiNon3GppRegistrationRequest = apiNon3GppRegistrationRequest.AmfNon3GppAccessRegistration(registrationData)
		_, httpResp, localErr := client.AMFRegistrationForNon3GPPAccessAPI.Non3GppRegistrationExecute(apiNon3GppRegistrationRequest)
		if localErr == nil {
			return nil, nil
		} else if httpResp != nil {
			if httpResp.Status != localErr.Error() {
				return nil, localErr
			}
			if problem, ok := openapi.ErrorModel[models.ProblemDetails](localErr); ok {
				return &problem, nil
			}
			return nil, localErr
		} else {
			return nil, openapi.ReportError("server no response")
		}
	}

	return nil, nil
}
