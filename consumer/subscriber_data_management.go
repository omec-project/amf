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
	"github.com/omec-project/openapi/v2/Nudm_SDM"
	"github.com/omec-project/openapi/v2/models"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

var tracer = otel.Tracer("amf/consumer")

func PutUpuAck(ctx context.Context, ue *amf_context.AmfUe, upuMacIue string) error {
	ctx, span := tracer.Start(ctx, "HTTP PUT udm/{supi}/am-data/upu-ack")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "PUT"),
		attribute.String("nf.target", "udm"),
		attribute.String("net.peer.name", ue.NudmSDMUri),
		attribute.String("udm.supi", ue.GetSupi()),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
		attribute.String("upu.mac.iue", upuMacIue),
	)

	configuration := Nudm_SDM.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.NudmSDMUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nudm_SDM.NewAPIClient(configuration)

	ackInfo := models.AcknowledgeInfo{
		UpuMacIue: openapi.PtrString(upuMacIue),
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	apiUpuAckRequest := client.ProvidingAcknowledgementOfUEParametersUpdateAPI.UpuAck(ctx, ue.GetSupi())
	apiUpuAckRequest = apiUpuAckRequest.AcknowledgeInfo(ackInfo)
	_, err := client.ProvidingAcknowledgementOfUEParametersUpdateAPI.UpuAckExecute(apiUpuAckRequest)
	return err
}

func SDMGetAmData(ctx context.Context, ue *amf_context.AmfUe) (problemDetails *models.ProblemDetails, err error) {
	ctx, span := tracer.Start(ctx, "HTTP GET udm/{supi}/am-data")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "GET"),
		attribute.String("nf.target", "udm"),
		attribute.String("net.peer.name", ue.NudmSDMUri),
		attribute.String("udm.supi", ue.GetSupi()),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Nudm_SDM.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.NudmSDMUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nudm_SDM.NewAPIClient(configuration)

	plmnId := models.NewPlmnIdNid(ue.PlmnId.Mcc, ue.PlmnId.Mnc)

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	apiGetAmDataRequest := client.AccessAndMobilitySubscriptionDataRetrievalAPI.GetAmData(ctx, ue.GetSupi())
	apiGetAmDataRequest = apiGetAmDataRequest.PlmnId(*plmnId)
	data, httpResp, localErr := client.AccessAndMobilitySubscriptionDataRetrievalAPI.GetAmDataExecute(apiGetAmDataRequest)
	if localErr == nil {
		ue.AccessAndMobilitySubscriptionData = data
		if len(data.Gpsis) > 0 {
			ue.SetGpsi(data.Gpsis[0]) // TODO: select GPSI
		} else {
			ue.SetGpsi("") // No GPSI associated with the UE, so clearing GPSI to avoid stale values
		}
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

func SDMGetSmfSelectData(ctx context.Context, ue *amf_context.AmfUe) (problemDetails *models.ProblemDetails, err error) {
	ctx, span := tracer.Start(ctx, "HTTP GET udm/{supi}/smf-select-data")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "GET"),
		attribute.String("nf.target", "udm"),
		attribute.String("net.peer.name", ue.NudmSDMUri),
		attribute.String("udm.supi", ue.GetSupi()),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Nudm_SDM.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.NudmSDMUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nudm_SDM.NewAPIClient(configuration)

	plmnId := models.NewPlmnId(ue.PlmnId.Mcc, ue.PlmnId.Mnc)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	apiGetSmfSelDataRequest := client.SMFSelectionSubscriptionDataRetrievalAPI.GetSmfSelData(ctx, ue.GetSupi())
	apiGetSmfSelDataRequest = apiGetSmfSelDataRequest.PlmnId(*plmnId)
	data, httpResp, localErr := client.SMFSelectionSubscriptionDataRetrievalAPI.GetSmfSelDataExecute(apiGetSmfSelDataRequest)
	if localErr == nil {
		ue.SmfSelectionData = data
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

func SDMGetUeContextInSmfData(ctx context.Context, ue *amf_context.AmfUe) (problemDetails *models.ProblemDetails, err error) {
	ctx, span := tracer.Start(ctx, "HTTP GET udm/{supi}/ue-context-in-smf-data")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "GET"),
		attribute.String("nf.target", "udm"),
		attribute.String("net.peer.name", ue.NudmSDMUri),
		attribute.String("udm.supi", ue.GetSupi()),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Nudm_SDM.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.NudmSDMUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nudm_SDM.NewAPIClient(configuration)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	apiGetUeCtxInSmfDataRequest := client.UEContextInSMFDataRetrievalAPI.GetUeCtxInSmfData(ctx, ue.GetSupi())
	data, httpResp, localErr := client.UEContextInSMFDataRetrievalAPI.GetUeCtxInSmfDataExecute(apiGetUeCtxInSmfDataRequest)
	if localErr == nil {
		ue.UeContextInSmfData = data
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

func SDMSubscribe(ctx context.Context, ue *amf_context.AmfUe) (problemDetails *models.ProblemDetails, err error) {
	ctx, span := tracer.Start(ctx, "HTTP POST udm/{supi}/sdm-subscriptions")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("nf.target", "udm"),
		attribute.String("net.peer.name", ue.NudmSDMUri),
		attribute.String("udm.supi", ue.GetSupi()),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Nudm_SDM.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.NudmSDMUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nudm_SDM.NewAPIClient(configuration)

	amfSelf := amf_context.AMF_Self()
	sdmSubscription := models.SdmSubscription{
		NfInstanceId: amfSelf.NfId,
		PlmnId:       &ue.PlmnId,
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	apiSubscribeRequest := client.SubscriptionCreationAPI.Subscribe(ctx, ue.GetSupi())
	apiSubscribeRequest = apiSubscribeRequest.SdmSubscription(sdmSubscription)
	_, httpResp, localErr := client.SubscriptionCreationAPI.SubscribeExecute(apiSubscribeRequest)
	if localErr == nil {
		return nil, nil
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

func SDMGetSliceSelectionSubscriptionData(ctx context.Context, ue *amf_context.AmfUe) (problemDetails *models.ProblemDetails, err error) {
	ctx, span := tracer.Start(ctx, "HTTP GET udm/{supi}/nssai")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "GET"),
		attribute.String("nf.target", "udm"),
		attribute.String("net.peer.name", ue.NudmSDMUri),
		attribute.String("udm.supi", ue.GetSupi()),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Nudm_SDM.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = ue.NudmSDMUri
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	client := Nudm_SDM.NewAPIClient(configuration)

	plmnId := models.NewPlmnId(ue.PlmnId.Mcc, ue.PlmnId.Mnc)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	apiGetNSSAIRequest := client.SliceSelectionSubscriptionDataRetrievalAPI.GetNSSAI(ctx, ue.GetSupi())
	apiGetNSSAIRequest = apiGetNSSAIRequest.PlmnId(*plmnId)
	nssai, httpResp, localErr := client.SliceSelectionSubscriptionDataRetrievalAPI.GetNSSAIExecute(apiGetNSSAIRequest)
	if localErr == nil {
		for _, defaultSnssai := range nssai.DefaultSingleNssais {
			subscribedSnssai := models.SubscribedSnssai{
				SubscribedSnssai: models.Snssai{
					Sst: defaultSnssai.Sst,
					Sd:  defaultSnssai.Sd,
				},
				DefaultIndication: openapi.PtrBool(true),
			}
			ue.SubscribedNssai = append(ue.SubscribedNssai, subscribedSnssai)
		}
		for _, snssai := range nssai.SingleNssais {
			subscribedSnssai := models.SubscribedSnssai{
				SubscribedSnssai: models.Snssai{
					Sst: snssai.Sst,
					Sd:  snssai.Sd,
				},
				DefaultIndication: openapi.PtrBool(false),
			}
			ue.SubscribedNssai = append(ue.SubscribedNssai, subscribedSnssai)
		}
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
		err = openapi.ReportError("Could not contact UDM at %v, %+v", ue.NudmSDMUri, localErr)
	}
	return problemDetails, err
}
