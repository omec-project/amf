// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"regexp"
	"time"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Npcf_AMPolicyControl"
	"github.com/omec-project/openapi/v2/models"
	"go.opentelemetry.io/otel/attribute"
)

func AMPolicyControlCreate(ctx context.Context, ue *amf_context.AmfUe, anType models.AccessType) (*models.ProblemDetails, error) {
	ctx, span := tracer.Start(ctx, "HTTP POST pcf/policies")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "pcf"),
		attribute.String("net.peer.name", ue.PcfUri),
		attribute.String("ue.supi", ue.GetSupi()),
		attribute.String("ue.plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Npcf_AMPolicyControl.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.PcfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Npcf_AMPolicyControl.NewAPIClient(configuration)

	amfSelf := amf_context.AMF_Self()

	policyAssociationRequest := models.PolicyAssociationRequest{
		NotificationUri: amfSelf.GetIPv4Uri() + "/namf-callback/v1/am-policy/",
		Supi:            ue.GetSupi(),
		AccessType:      &anType,
		ServingPlmn:     models.NewPlmnIdNid(ue.PlmnId.GetMcc(), ue.PlmnId.GetMnc()),
		Guami:           &amfSelf.ServedGuamiList[0],
	}
	if ue.GetPei() != "" {
		policyAssociationRequest.Pei = openapi.PtrString(ue.GetPei())
	}
	if ue.GetGpsi() != "" {
		policyAssociationRequest.Gpsi = openapi.PtrString(ue.GetGpsi())
	}

	if ue.AccessAndMobilitySubscriptionData != nil {
		policyAssociationRequest.Rfsp = ue.AccessAndMobilitySubscriptionData.RfspIndex.Get()
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	apiCreateIndividualAMPolicyAssociationRequest := client.AMPolicyAssociationsCollectionAPI.CreateIndividualAMPolicyAssociation(ctx)
	apiCreateIndividualAMPolicyAssociationRequest = apiCreateIndividualAMPolicyAssociationRequest.PolicyAssociationRequest(policyAssociationRequest)
	res, httpResp, localErr := client.AMPolicyAssociationsCollectionAPI.CreateIndividualAMPolicyAssociationExecute(apiCreateIndividualAMPolicyAssociationRequest)
	if localErr == nil {
		locationHeader := httpResp.Header.Get("Location")
		logger.ConsumerLog.Debugf("location header: %+v", locationHeader)
		ue.AmPolicyUri = locationHeader

		re := regexp.MustCompile("/policies/.*")
		match := re.FindStringSubmatch(locationHeader)

		ue.PolicyAssociationId = match[0][10:]
		ue.AmPolicyAssociation = res

		if res.Triggers != nil {
			for _, trigger := range res.Triggers {
				if trigger == models.REQUESTTRIGGER_LOC_CH {
					ue.RequestTriggerLocationChange = true
				}
				//if trigger == models.REQUESTTRIGGER_PRA_CH {
				// TODO: Presence Reporting Area handling (TS 23.503 6.1.2.5, TS 23.501 5.6.11)
				//}
			}
		}

		logger.ConsumerLog.Debugf("UE AM Policy Association ID: %s", ue.PolicyAssociationId)
		logger.ConsumerLog.Debugf("AmPolicyAssociation: %+v", ue.AmPolicyAssociation)
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
	return nil, nil
}

func AMPolicyControlUpdate(ctx context.Context, ue *amf_context.AmfUe, updateRequest models.PolicyAssociationUpdateRequest) (
	problemDetails *models.ProblemDetails, err error,
) {
	ctx, span := tracer.Start(ctx, "HTTP POST pcf/policies/{polAssoId}/update")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "pcf"),
		attribute.String("net.peer.name", ue.PcfUri),
		attribute.String("ue.supi", ue.GetSupi()),
		attribute.String("ue.plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Npcf_AMPolicyControl.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.PcfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Npcf_AMPolicyControl.NewAPIClient(configuration)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	apiReportObservedEventTriggersForIndividualAMPolicyAssociationRequest := client.IndividualAMPolicyAssociationDocumentAPI.ReportObservedEventTriggersForIndividualAMPolicyAssociation(ctx, ue.PolicyAssociationId)
	apiReportObservedEventTriggersForIndividualAMPolicyAssociationRequest = apiReportObservedEventTriggersForIndividualAMPolicyAssociationRequest.PolicyAssociationUpdateRequest(updateRequest)
	res, httpResp, localErr := client.IndividualAMPolicyAssociationDocumentAPI.ReportObservedEventTriggersForIndividualAMPolicyAssociationExecute(apiReportObservedEventTriggersForIndividualAMPolicyAssociationRequest)
	if localErr == nil {
		if res.ServAreaRes != nil {
			ue.AmPolicyAssociation.ServAreaRes = res.ServAreaRes
		}
		if res.GetRfsp() != 0 {
			ue.AmPolicyAssociation.Rfsp = res.Rfsp
		}
		ue.AmPolicyAssociation.Triggers = res.Triggers
		ue.RequestTriggerLocationChange = false
		for _, trigger := range res.Triggers {
			if trigger == models.REQUESTTRIGGER_LOC_CH {
				ue.RequestTriggerLocationChange = true
			}
			// if trigger == models.REQUESTTRIGGER_PRA_CH {
			// TODO: Presence Reporting Area handling (TS 23.503 6.1.2.5, TS 23.501 5.6.11)
			// }
		}
		return problemDetails, err
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			err = localErr
			return problemDetails, err
		}
		if problem, ok := openapi.ErrorModel[models.ProblemDetails](localErr); ok {
			problemDetails = &problem
		} else {
			err = localErr
		}
	} else {
		err = openapi.ReportError("server no response")
	}
	return problemDetails, err
}

func AMPolicyControlDelete(ctx context.Context, ue *amf_context.AmfUe) (problemDetails *models.ProblemDetails, err error) {
	ctx, span := tracer.Start(ctx, "HTTP DELETE pcf/policies/{polAssoId}")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "DELETE"),
		attribute.String("nf.target", "pcf"),
		attribute.String("net.peer.name", ue.PcfUri),
		attribute.String("ue.supi", ue.GetSupi()),
		attribute.String("ue.plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Npcf_AMPolicyControl.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.PcfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Npcf_AMPolicyControl.NewAPIClient(configuration)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	apiDeleteIndividualAMPolicyAssociationRequest := client.IndividualAMPolicyAssociationDocumentAPI.DeleteIndividualAMPolicyAssociation(ctx, ue.PolicyAssociationId)
	httpResp, localErr := client.IndividualAMPolicyAssociationDocumentAPI.DeleteIndividualAMPolicyAssociationExecute(apiDeleteIndividualAMPolicyAssociationRequest)
	if localErr == nil {
		ue.RemoveAmPolicyAssociation()
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			err = localErr
			return problemDetails, err
		}
		if problem, ok := openapi.ErrorModel[models.ProblemDetails](localErr); ok {
			problemDetails = &problem
		} else {
			err = localErr
		}
	} else {
		err = openapi.ReportError("server no response")
	}

	return problemDetails, err
}
