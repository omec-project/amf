// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package consumer

import (
	"context"
	"time"

	"github.com/antihax/optional"
	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/openapi"
	"github.com/omec-project/openapi/Nudm_SubscriberDataManagement"
	"github.com/omec-project/openapi/models"
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
		attribute.String("udm.supi", ue.Supi),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
		attribute.String("upu.mac.iue", upuMacIue),
	)

	configuration := Nudm_SubscriberDataManagement.NewConfiguration()
	configuration.SetBasePath(ue.NudmSDMUri)
	client := Nudm_SubscriberDataManagement.NewAPIClient(configuration)

	ackInfo := models.AcknowledgeInfo{
		UpuMacIue: upuMacIue,
	}
	upuOpt := Nudm_SubscriberDataManagement.PutUpuAckParamOpts{
		AcknowledgeInfo: optional.NewInterface(ackInfo),
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, err := client.ProvidingAcknowledgementOfUEParametersUpdateApi.PutUpuAck(ctx, ue.Supi, &upuOpt)
	return err
}

func SDMGetAmData(ctx context.Context, ue *amf_context.AmfUe) (problemDetails *models.ProblemDetails, err error) {
	ctx, span := tracer.Start(ctx, "HTTP GET udm/{supi}/am-data")
	defer span.End()

	span.SetAttributes(
		attribute.String("http.method", "GET"),
		attribute.String("nf.target", "udm"),
		attribute.String("net.peer.name", ue.NudmSDMUri),
		attribute.String("udm.supi", ue.Supi),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Nudm_SubscriberDataManagement.NewConfiguration()
	configuration.SetBasePath(ue.NudmSDMUri)
	client := Nudm_SubscriberDataManagement.NewAPIClient(configuration)

	getAmDataParamOpt := Nudm_SubscriberDataManagement.GetAmDataParamOpts{
		PlmnId: optional.NewInterface(ue.PlmnId.Mcc + ue.PlmnId.Mnc),
	}

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	data, httpResp, localErr := client.AccessAndMobilitySubscriptionDataRetrievalApi.GetAmData(
		ctx, ue.Supi, &getAmDataParamOpt)
	if localErr == nil {
		ue.AccessAndMobilitySubscriptionData = &data
		ue.Gpsi = data.Gpsis[0] // TODO: select GPSI
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			err = localErr
			return problemDetails, err
		}
		problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
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
		attribute.String("udm.supi", ue.Supi),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Nudm_SubscriberDataManagement.NewConfiguration()
	configuration.SetBasePath(ue.NudmSDMUri)
	client := Nudm_SubscriberDataManagement.NewAPIClient(configuration)

	paramOpt := Nudm_SubscriberDataManagement.GetSmfSelectDataParamOpts{
		PlmnId: optional.NewInterface(ue.PlmnId.Mcc + ue.PlmnId.Mnc),
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	data, httpResp, localErr := client.SMFSelectionSubscriptionDataRetrievalApi.GetSmfSelectData(ctx, ue.Supi, &paramOpt)
	if localErr == nil {
		ue.SmfSelectionData = &data
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			err = localErr
			return problemDetails, err
		}
		problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
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
		attribute.String("udm.supi", ue.Supi),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Nudm_SubscriberDataManagement.NewConfiguration()
	configuration.SetBasePath(ue.NudmSDMUri)
	client := Nudm_SubscriberDataManagement.NewAPIClient(configuration)
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	data, httpResp, localErr := client.UEContextInSMFDataRetrievalApi.GetUeContextInSmfData(ctx, ue.Supi, nil)
	if localErr == nil {
		ue.UeContextInSmfData = &data
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			err = localErr
			return problemDetails, err
		}
		problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
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
		attribute.String("udm.supi", ue.Supi),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Nudm_SubscriberDataManagement.NewConfiguration()
	configuration.SetBasePath(ue.NudmSDMUri)
	client := Nudm_SubscriberDataManagement.NewAPIClient(configuration)

	amfSelf := amf_context.AMF_Self()
	sdmSubscription := models.SdmSubscription{
		NfInstanceId: amfSelf.NfId,
		PlmnId:       &ue.PlmnId,
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	_, httpResp, localErr := client.SubscriptionCreationApi.Subscribe(ctx, ue.Supi, sdmSubscription)
	if localErr == nil {
		return nil, nil
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			err = localErr
			return problemDetails, err
		}
		problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
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
		attribute.String("udm.supi", ue.Supi),
		attribute.String("plmn.id", ue.PlmnId.Mcc+ue.PlmnId.Mnc),
	)

	configuration := Nudm_SubscriberDataManagement.NewConfiguration()
	configuration.SetBasePath(ue.NudmSDMUri)
	client := Nudm_SubscriberDataManagement.NewAPIClient(configuration)

	paramOpt := Nudm_SubscriberDataManagement.GetNssaiParamOpts{
		PlmnId: optional.NewInterface(ue.PlmnId.Mcc + ue.PlmnId.Mnc),
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	nssai, httpResp, localErr := client.SliceSelectionSubscriptionDataRetrievalApi.GetNssai(ctx, ue.Supi, &paramOpt)
	if localErr == nil {
		for _, defaultSnssai := range nssai.DefaultSingleNssais {
			subscribedSnssai := models.SubscribedSnssai{
				SubscribedSnssai: &models.Snssai{
					Sst: defaultSnssai.Sst,
					Sd:  defaultSnssai.Sd,
				},
				DefaultIndication: true,
			}
			ue.SubscribedNssai = append(ue.SubscribedNssai, subscribedSnssai)
		}
		for _, snssai := range nssai.SingleNssais {
			subscribedSnssai := models.SubscribedSnssai{
				SubscribedSnssai: &models.Snssai{
					Sst: snssai.Sst,
					Sd:  snssai.Sd,
				},
				DefaultIndication: false,
			}
			ue.SubscribedNssai = append(ue.SubscribedNssai, subscribedSnssai)
		}
	} else if httpResp != nil {
		if httpResp.Status != localErr.Error() {
			err = localErr
			return problemDetails, err
		}
		problem := localErr.(openapi.GenericOpenAPIError).Model().(models.ProblemDetails)
		problemDetails = &problem
	} else {
		err = openapi.ReportError("Could not contact UDM at %v, %+v", ue.NudmSDMUri, localErr)
	}
	return problemDetails, err
}
