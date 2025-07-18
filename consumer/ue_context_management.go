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
	"github.com/omec-project/openapi"
	"github.com/omec-project/openapi/Nudm_UEContextManagement"
	"github.com/omec-project/openapi/models"
	"go.opentelemetry.io/otel/attribute"
)

func UeCmRegistration(ctx context.Context, ue *amf_context.AmfUe, accessType models.AccessType, initialRegistrationInd bool) (
	*models.ProblemDetails, error,
) {
	configuration := Nudm_UEContextManagement.NewConfiguration()
	configuration.SetBasePath(ue.NudmUECMUri)
	client := Nudm_UEContextManagement.NewAPIClient(configuration)

	amfSelf := amf_context.AMF_Self()

	switch accessType {
	case models.AccessType__3_GPP_ACCESS:
		registrationData := models.Amf3GppAccessRegistration{
			AmfInstanceId:          amfSelf.NfId,
			InitialRegistrationInd: initialRegistrationInd,
			Guami:                  &amfSelf.ServedGuamiList[0],
			RatType:                ue.RatType,
			ImsVoPs:                models.ImsVoPs_HOMOGENEOUS_NON_SUPPORT,
		}
		gppAccessCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		gppAccessCtx, span := tracer.Start(gppAccessCtx, "HTTP PUT udm/{ueId}/registrations/amf-3gpp-access")
		defer span.End()

		span.SetAttributes(
			attribute.String("http.method", "PUT"),
			attribute.String("nf.target", "udm"),
			attribute.String("net.peer.name", ue.NudmUECMUri),
			attribute.String("udm.supi", ue.Supi),
			attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
		)

		_, httpResp, localErr := client.AMFRegistrationFor3GPPAccessApi.Registration(gppAccessCtx, ue.Supi, registrationData)
		if localErr == nil {
			return nil, nil
		} else if httpResp != nil {
			if httpResp.Status != localErr.Error() {
				return nil, localErr
			}
			problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
			return &problem, nil
		} else {
			return nil, openapi.ReportError("server no response")
		}

	case models.AccessType_NON_3_GPP_ACCESS:
		registrationData := models.AmfNon3GppAccessRegistration{
			AmfInstanceId: amfSelf.NfId,
			Guami:         &amfSelf.ServedGuamiList[0],
			RatType:       ue.RatType,
		}

		non3gppAccessCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()

		non3gppAccessCtx, span := tracer.Start(non3gppAccessCtx, "HTTP PUT udm/{ueId}/registrations/amf-non-3gpp-access")
		defer span.End()

		span.SetAttributes(
			attribute.String("http.method", "PUT"),
			attribute.String("nf.target", "udm"),
			attribute.String("net.peer.name", ue.NudmUECMUri),
			attribute.String("udm.supi", ue.Supi),
			attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
		)

		_, httpResp, localErr := client.AMFRegistrationForNon3GPPAccessApi.Register(non3gppAccessCtx, ue.Supi, registrationData)
		if localErr == nil {
			return nil, nil
		} else if httpResp != nil {
			if httpResp.Status != localErr.Error() {
				return nil, localErr
			}
			problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
			return &problem, nil
		} else {
			return nil, openapi.ReportError("server no response")
		}
	}

	return nil, nil
}
