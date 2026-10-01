// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	ctxt "context"
	"net/url"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/httpwrapper"
)

// removedAfterLookup returns a UE that a request has already found but that was removed
// before the request was dispatched: it is still where the lookup finds it, and its event
// channel is created stopped.
func removedAfterLookup(t *testing.T, supi string) *context.AmfUe {
	t.Helper()

	ue := newUe(t, supi, false)
	ue.Remove()
	context.AMF_Self().UePool.Store(supi, ue)
	t.Cleanup(func() { context.AMF_Self().UePool.Delete(supi) })

	return ue
}

// DispatchSbiMsg answers a request for a removed UE with a context-not-found problem,
// and every service handler has to pass that on. One read only the procedure's own
// error type and reported 201 Created with no body instead.
func TestEveryServiceRequestForARemovedUeIsAnsweredWithAnError(t *testing.T) {
	disableKafkaForTest(t)

	ueContextRelease := models.UEContextRelease{}
	ueContextRelease.SetNgapCause(models.NgApCause{Group: 1, Value: 1})
	deregistrationData := models.DeregistrationData{}
	deregistrationData.SetDeregReason("SUBSCRIPTION_WITHDRAWN")

	tests := []struct {
		name   string
		supi   string
		handle func(ue *context.AmfUe) *httpwrapper.Response
	}{
		{"SmContextStatusNotify", "imsi-208930100007701", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleSmContextStatusNotify(&httpwrapper.Request{
				Params: map[string]string{"guti": ue.GetGuti(), "pduSessionId": "1"},
				Body:   models.SmContextStatusNotification{},
			})
		}},
		{"ProvideLocationInfo", "imsi-208930100007702", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleProvideLocationInfoRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
				Body:   *models.NewRequestLocInfo(),
			})
		}},
		{"ProvideDomainSelectionInfo", "imsi-208930100007703", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleProvideDomainSelectionInfoRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
			})
		}},
		{"ReleaseUEContext", "imsi-208930100007704", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleReleaseUEContextRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
				Body:   ueContextRelease,
			})
		}},
		{"DeregistrationNotification", "imsi-208930100007705", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleDeregistrationNotification(ctxt.Background(), &httpwrapper.Request{
				Params: map[string]string{"supi": ue.GetSupi()},
				Body:   deregistrationData,
				URL:    &url.URL{Path: "/namf-callback/v1/deregistration/" + ue.GetSupi()},
			})
		}},
		{"CreateUEContext", "imsi-208930100007706", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleCreateUEContextRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
				Body:   models.CreateUEContextRequest{},
			})
		}},
		{"UEContextTransfer", "imsi-208930100007707", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleUEContextTransferRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
				Body:   models.UEContextTransferRequest{},
			})
		}},
		{"AssignEbiData", "imsi-208930100007708", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleAssignEbiDataRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
				Body:   models.AssignEbiData{},
			})
		}},
		{"RegistrationStatusUpdate", "imsi-208930100007709", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleRegistrationStatusUpdateRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
				Body:   models.UeRegStatusUpdateReqData{},
			})
		}},
		{"N1N2MessageTransfer", "imsi-208930100007710", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleN1N2MessageTransferRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi(), paramReqURI: n1n2MessagesURI},
				Body:   models.N1N2MessageTransferRequest{JsonData: models.NewN1N2MessageTransferReqData()},
			})
		}},
		{"N1N2MessageTransferStatus", "imsi-208930100007711", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleN1N2MessageTransferStatusRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi(), paramReqURI: "/n1-n2-messages/1"},
			})
		}},
		{"N1N2MessageSubscribe", "imsi-208930100007712", func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleN1N2MessageSubscirbeRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
				Body:   models.UeN1N2InfoSubscriptionCreateData{},
			})
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := tc.handle(removedAfterLookup(t, tc.supi))
			if response == nil {
				t.Fatal("no response")
				return
			}
			if response.Status >= 200 && response.Status < 300 {
				t.Fatalf("status %d for a UE removed before the request was handled; want an error",
					response.Status)
			}
			// The dispatch's own answer, not some earlier rejection of the request: a
			// handler that refused the body before dispatching would pass the status check
			// without ever reaching the path this is about.
			problem, ok := response.Body.(*models.ProblemDetails)
			if !ok || problem.GetDetail() != "UE context removed" {
				t.Fatalf("status %d, body %#v: want the dispatch's context-not-found answer",
					response.Status, response.Body)
			}
		})
	}
}
