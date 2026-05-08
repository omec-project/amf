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
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi"
	"github.com/omec-project/openapi/Nnssf_NSSelection"
	"github.com/omec-project/openapi/models"
	"go.opentelemetry.io/otel/attribute"
)

func NSSelectionGetForRegistration(ctx context.Context, ue *amf_context.AmfUe, requestedNssai []models.MappingOfSnssai) (
	*models.ProblemDetails, error,
) {
	ctx, span := tracer.Start(ctx, "HTTP GET nssf/network-slice-information")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "GET"),
		attribute.String("nf.target", "nssf"),
		attribute.String("net.peer.name", ue.NssfUri),
		attribute.String("amf.nf.id", amf_context.AMF_Self().NfId),
	)

	configuration := Nnssf_NSSelection.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.NssfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nnssf_NSSelection.NewAPIClient(configuration)

	amfSelf := amf_context.AMF_Self()
	sliceInfo := models.SliceInfoForRegistration{
		SubscribedNssai: ue.SubscribedNssai,
	}

	for _, snssai := range requestedNssai {
		sliceInfo.RequestedNssai = append(sliceInfo.RequestedNssai, snssai.ServingSnssai)
		if snssai.HomeSnssai.GetSst() != 0 {
			sliceInfo.MappingOfNssai = append(sliceInfo.MappingOfNssai, snssai)
		}
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	apiNSSelectionGetRequest := client.NetworkSliceInformationDocumentAPI.NSSelectionGet(ctx)
	apiNSSelectionGetRequest = apiNSSelectionGetRequest.NfType(models.NFTYPE_AMF)
	apiNSSelectionGetRequest = apiNSSelectionGetRequest.NfId(amfSelf.NfId)
	apiNSSelectionGetRequest = apiNSSelectionGetRequest.SliceInfoRequestForRegistration(sliceInfo)
	res, httpResp, localErr := client.NetworkSliceInformationDocumentAPI.NSSelectionGetExecute(apiNSSelectionGetRequest)
	if localErr == nil {
		ue.NetworkSliceInfo = res
		for _, allowedNssai := range res.AllowedNssaiList {
			ue.AllowedNssai[allowedNssai.AccessType] = allowedNssai.AllowedSnssaiList
		}
		ue.ConfiguredNssai = res.ConfiguredNssai
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			err := localErr
			return nil, err
		}
		if problem, ok := openapi.ErrorModel[models.ProblemDetails](localErr); ok {
			return &problem, nil
		}
		return nil, localErr
	} else {
		return nil, openapi.ReportError("NSSF No Response")
	}

	return nil, nil
}

func NSSelectionGetForPduSession(ctx context.Context, ue *amf_context.AmfUe, snssai models.Snssai) (
	*models.AuthorizedNetworkSliceInfo, *models.ProblemDetails, error,
) {
	logger.ConsumerLog.Infoln("NSSelectionGetForPduSession")
	ctx, span := tracer.Start(ctx, "HTTP GET nssf/network-slice-information")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "GET"),
		attribute.String("nf.target", "nssf"),
		attribute.String("net.peer.name", ue.NssfUri),
		attribute.String("amf.nf.id", amf_context.AMF_Self().NfId),
		attribute.Int("snssai.sst", int(snssai.Sst)),
		attribute.String("snssai.sd", snssai.GetSd()),
	)

	configuration := Nnssf_NSSelection.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.NssfUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nnssf_NSSelection.NewAPIClient(configuration)

	amfSelf := amf_context.AMF_Self()
	sliceInfoForPduSession := models.SliceInfoForPDUSession{
		SNssai:            snssai,
		RoamingIndication: models.ROAMINGINDICATION_NON_ROAMING, // not support roaming
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	apiNSSelectionGetRequest := client.NetworkSliceInformationDocumentAPI.NSSelectionGet(ctx)
	apiNSSelectionGetRequest = apiNSSelectionGetRequest.NfType(models.NFTYPE_AMF)
	apiNSSelectionGetRequest = apiNSSelectionGetRequest.NfId(amfSelf.NfId)
	apiNSSelectionGetRequest = apiNSSelectionGetRequest.SliceInfoRequestForPduSession(sliceInfoForPduSession)
	res, httpResp, localErr := client.NetworkSliceInformationDocumentAPI.NSSelectionGetExecute(apiNSSelectionGetRequest)
	if localErr == nil {
		return res, nil, nil
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			return nil, nil, localErr
		}
		if problem, ok := openapi.ErrorModel[models.ProblemDetails](localErr); ok {
			return nil, &problem, nil
		}
		return nil, nil, localErr
	} else {
		return nil, nil, openapi.ReportError("NSSF No Response")
	}
}
