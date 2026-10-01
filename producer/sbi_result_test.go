// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/httpwrapper"
)

// The SBI handlers return their results through interface{}, and a nil *T in an interface is
// not nil. Every caller tests the field it was handed, so before anyOrNil each of these
// answered for a condition that had not occurred. The statuses below are what the handler
// code plainly reads as its own success and failure paths.
//
// Mutation check: make anyOrNil return p unconditionally and all four fail, each with the
// status recorded in its name.

// resultUe is newUe's live-UE fixture: these exercise the normal event-channel dispatch
// path, not the restored-UE fallback.
func resultUe(t *testing.T, supi string) *context.AmfUe {
	t.Helper()

	disableKafkaForTest(t)

	return newUe(t, supi, true)
}

// The handler's own success path returns http.StatusNoContent. It answered 0, because the
// caller read the status of a nil *ProblemDetails instead of reaching that path.
func TestReleaseUeContextAnswers204OnSuccess(t *testing.T) {
	ue := resultUe(t, "imsi-208930100009201")

	ueContextRelease := models.UEContextRelease{}
	ueContextRelease.SetNgapCause(models.NgApCause{Group: 1, Value: 1})

	response := HandleReleaseUEContextRequest(&httpwrapper.Request{
		Params: map[string]string{paramUeContextID: ue.GetSupi()},
		Body:   ueContextRelease,
	})

	if response.Status != 204 {
		t.Fatalf("status = %d, want 204", response.Status)
	}
}

// A successful ProvideLocationInfo is 200. It answered 500: the caller mapped the nil
// problem details' status of 0 onto InternalServerError.
func TestProvideLocationInfoAnswers200OnSuccess(t *testing.T) {
	ue := resultUe(t, "imsi-208930100009202")

	response := HandleProvideLocationInfoRequest(&httpwrapper.Request{
		Params: map[string]string{paramUeContextID: ue.GetSupi()},
		Body:   *models.NewRequestLocInfo(),
	})

	if response.Status != 200 {
		t.Fatalf("status = %d, want 200", response.Status)
	}
}

// Same shape on the MT side.
func TestProvideDomainSelectionInfoAnswers200OnSuccess(t *testing.T) {
	ue := resultUe(t, "imsi-208930100009203")

	response := HandleProvideDomainSelectionInfoRequest(&httpwrapper.Request{
		Params: map[string]string{paramUeContextID: ue.GetSupi()},
	})

	if response.Status != 200 {
		t.Fatalf("status = %d, want 200", response.Status)
	}
}

// The worst of the three, because it inverts the answer rather than garbling it: the caller
// type-asserts RespData, a typed nil asserts successfully with ok == true, and a *failed*
// registration status update was reported to the peer AMF as 200. A context id that is not a
// 5G-GUTI is the procedure's own rejection, per the comment on that check.
func TestRegistrationStatusUpdateReportsFailureNotSuccess(t *testing.T) {
	ue := resultUe(t, "imsi-208930100009204")

	response := HandleRegistrationStatusUpdateRequest(&httpwrapper.Request{
		Params: map[string]string{paramUeContextID: ue.GetSupi()},
		Body:   models.UeRegStatusUpdateReqData{},
	})

	if response.Status == 200 {
		t.Fatal("a rejected registration status update was reported as 200 success")
	}
	problemDetails, ok := response.Body.(*models.ProblemDetails)
	if !ok || problemDetails == nil {
		t.Fatalf("body = %#v, want the procedure's *models.ProblemDetails", response.Body)
	}
}

// HandleN1N2MessageTransferStatusRequest panicked on every call through the channel path:
// N1N2MessageTransferStatusProcedure returns models.N1N2MessageTransferCause, a string type,
// and boxing that value gives an interface that is never nil and that the caller's
// .(*models.N1N2MessageTransferCause) assertion cannot satisfy. gin recovers it as a 500, so
// the endpoint has been answering 500 rather than crashing. Found by the sweep for the
// typed-nil defect above -- the same return statement, the opposite mistake.
func TestN1N2MessageTransferStatusDoesNotPanic(t *testing.T) {
	ue := resultUe(t, "imsi-208930100009301")

	// The procedure answers 404 unless the UE holds an N1N2 message whose ResourceUri
	// matches, so give it one -- otherwise this exercises the failure path and never
	// reaches the boxed cause at all.
	const reqURI = "/namf-comm/v1/ue-contexts/x/n1-n2-messages/1"
	ue.N1N2Message = &context.N1N2Message{
		Status:      models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE,
		ResourceUri: context.AMF_Self().GetIPv4Uri() + reqURI,
	}

	response := HandleN1N2MessageTransferStatusRequest(&httpwrapper.Request{
		Params: map[string]string{paramUeContextID: ue.GetSupi(), paramReqURI: reqURI},
	})

	if response == nil {
		t.Fatal("no response")
		return
	}
	if response.Status != 200 {
		t.Fatalf("status = %d, want 200", response.Status)
	}
	cause, ok := response.Body.(*models.N1N2MessageTransferCause)
	if !ok {
		t.Fatalf("body = %#v, want *models.N1N2MessageTransferCause", response.Body)
		return
	}
	if *cause != models.N1N2MESSAGETRANSFERCAUSE_ATTEMPTING_TO_REACH_UE {
		t.Fatalf("cause = %q, want the stored one", *cause)
	}
}
