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

// A UE context restored from the datastore is published into the UE pool with no
// event channel: the field is json:"-" and DbFetch clears it, while only the NAS and
// NGAP dispatchers create one. Each test below drives a service handler twice for the
// same request -- once for a UE that has a channel and once for a UE that does not --
// and asserts both are answered the same way, which is the part that matters: the
// answer a restored UE gets is the one its state warrants, not a recovered 500.
//
// One test per handler shape rather than one per call site. The twelve unguarded
// sites share five handlers between them: SmContextHandler, LocationInfoHandler,
// MtHandler, UeContextHandler and HandleOAMPurgeUEContextRequest.
//
// Mutation check: with DispatchSbiMsg's nil branch removed, the channel-less half of
// every one of these panics on the nil dereference.

// paramUeContextID is the path parameter the service handlers read the UE out of.
const paramUeContextID = "ueContextId"

// paramReqURI is the request parameter the N1N2 handlers read their resource URI from.
const paramReqURI = "reqUri"

// ranUeNgapID is arbitrary; each UE gets its own AmfRan, so they do not collide.
const ranUeNgapID = 1

// newUe puts a UE in the pool in the shape a restore leaves it: DbFetch calls
// SetAmfUe on the stored RanUe and stores it in the pool, then clears EventChannel, so
// a restored UE has its RAN context and nothing else. withChannel is the only
// difference between that and a UE the NGAP dispatcher is handling.
func newUe(t *testing.T, supi string, withChannel bool) *context.AmfUe {
	t.Helper()

	ue := context.AMF_Self().NewAmfUe(supi)

	ran := context.NewAmfRanDefault()
	// AttachRanUe keys ue.RanUe by ran.AnType, and DbFetch restores under the 3GPP key.
	// Without this the RanUe lands under the empty access type, GetAnType returns "", and
	// the handlers below take their no-access path instead of the one a restored UE hits.
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	ranUe, err := ran.NewRanUe(ranUeNgapID)
	if err != nil {
		t.Fatalf("could not create a RanUe: %v", err)
	}
	ue.AttachRanUe(ranUe)

	if withChannel {
		// What ngap/dispatcher.go does for a UE it is handling.
		ue.SetEventChannel(ctxt.Background(), func(*context.AmfUe, context.NgapMsg) {})
	} else {
		// What context/db.go leaves behind on a restore.
		ue.EventChannel = nil
	}
	// Conditional: the release and deregistration cases remove the UE inside the handler,
	// and Remove is not written to be called twice -- its second run walks an empty RanUe
	// map and logs "RanUe not found" at ERROR, which is noise this change's own
	// "happy path does not emit errors" requirement would object to.
	t.Cleanup(func() {
		if _, stillPooled := context.AMF_Self().AmfUeFindBySupiLocal(supi); stillPooled {
			ue.Remove()
		}
	})

	return ue
}

// runBothWays calls handle for a UE with an event channel and for one without, and
// fails unless both answered identically.
func runBothWays(t *testing.T, supiWith, supiWithout string,
	handle func(ue *context.AmfUe) *httpwrapper.Response,
) {
	t.Helper()

	// The release and deregistration paths publish a Del to Kafka before Remove (see
	// producer/ue_context.go), and there is no broker in a unit test. Same helper the
	// OAM tests in this package use.
	disableKafkaForTest(t)

	withChannel := handle(newUe(t, supiWith, true))
	if withChannel == nil {
		t.Fatal("the UE with an event channel was not answered at all")
	}

	withoutChannel := handle(newUe(t, supiWithout, false))
	if withoutChannel == nil {
		t.Fatal("the UE without an event channel was not answered at all")
	}

	if withoutChannel.Status != withChannel.Status {
		t.Fatalf("status without an event channel = %d, with one = %d",
			withoutChannel.Status, withChannel.Status)
	}
	if (withoutChannel.Body == nil) != (withChannel.Body == nil) {
		t.Fatalf("body without an event channel = %T, with one = %T",
			withoutChannel.Body, withChannel.Body)
	}
}

func TestSmContextStatusNotifyAnswersWithoutEventChannel(t *testing.T) {
	runBothWays(t, "imsi-208930100007601", "imsi-208930100007602",
		func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleSmContextStatusNotify(&httpwrapper.Request{
				Params: map[string]string{"guti": ue.GetGuti(), "pduSessionId": "1"},
				Body:   models.SmContextStatusNotification{},
			})
		})
}

func TestProvideLocationInfoAnswersWithoutEventChannel(t *testing.T) {
	runBothWays(t, "imsi-208930100007603", "imsi-208930100007604",
		func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleProvideLocationInfoRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
				Body:   *models.NewRequestLocInfo(),
			})
		})
}

func TestProvideDomainSelectionInfoAnswersWithoutEventChannel(t *testing.T) {
	runBothWays(t, "imsi-208930100007605", "imsi-208930100007606",
		func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleProvideDomainSelectionInfoRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
			})
		})
}

func TestReleaseUeContextAnswersWithoutEventChannel(t *testing.T) {
	ueContextRelease := models.UEContextRelease{}
	ueContextRelease.SetNgapCause(models.NgApCause{Group: 1, Value: 1})

	var released []string
	runBothWays(t, "imsi-208930100007607", "imsi-208930100007608",
		func(ue *context.AmfUe) *httpwrapper.Response {
			supi := ue.GetSupi()
			released = append(released, supi)
			return HandleReleaseUEContextRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: supi},
				Body:   ueContextRelease,
			})
		})

	// The status these handlers return on success is not a good witness that the
	// procedure ran -- it is 0 either way, because UeContextHandler hands back a
	// typed-nil *models.ProblemDetails that the caller reads as non-nil. The UE
	// leaving the pool is.
	for _, supi := range released {
		if _, ok := context.AMF_Self().AmfUeFindBySupiLocal(supi); ok {
			t.Fatalf("%s is still in the pool, so the release did not reach the procedure", supi)
		}
	}
}

func TestDeregistrationNotificationAnswersWithoutEventChannel(t *testing.T) {
	deregistrationData := models.DeregistrationData{}
	deregistrationData.SetDeregReason("SUBSCRIPTION_WITHDRAWN")

	var purged []string
	runBothWays(t, "imsi-208930100007609", "imsi-208930100007610",
		func(ue *context.AmfUe) *httpwrapper.Response {
			supi := ue.GetSupi()
			purged = append(purged, supi)
			return HandleDeregistrationNotification(ctxt.Background(), &httpwrapper.Request{
				Params: map[string]string{"supi": supi},
				Body:   deregistrationData,
				URL:    &url.URL{Path: "/namf-callback/v1/deregistration/" + supi},
			})
		})

	// Matching the release test, and for the same reason: 204 with an empty body is what a
	// fallback that quietly did nothing would also return. newUe starts Deregistered, so the
	// handler's observable work is removing the UE -- assert that, not the status.
	for _, supi := range purged {
		if _, ok := context.AMF_Self().AmfUeFindBySupiLocal(supi); ok {
			t.Fatalf("%s is still in the pool, so the purge did not reach the procedure", supi)
		}
	}
}

// The three N1N2 handlers used to guard a missing channel with a direct call of their
// own. They now go through DispatchSbiMsg like the rest, so a UE without a channel has
// one created and must be answered exactly as a UE that had one.

func TestN1N2MessageTransferAnswersWithoutEventChannel(t *testing.T) {
	runBothWays(t, "imsi-208930100007621", "imsi-208930100007622",
		func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleN1N2MessageTransferRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi(), paramReqURI: "/n1-n2-messages"},
				Body:   models.N1N2MessageTransferRequest{JsonData: models.NewN1N2MessageTransferReqData()},
			})
		})
}

func TestN1N2MessageTransferStatusAnswersWithoutEventChannel(t *testing.T) {
	runBothWays(t, "imsi-208930100007623", "imsi-208930100007624",
		func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleN1N2MessageTransferStatusRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi(), paramReqURI: "/n1-n2-messages/1"},
			})
		})
}

func TestN1N2MessageSubscribeAnswersWithoutEventChannel(t *testing.T) {
	runBothWays(t, "imsi-208930100007625", "imsi-208930100007626",
		func(ue *context.AmfUe) *httpwrapper.Response {
			return HandleN1N2MessageSubscirbeRequest(&httpwrapper.Request{
				Params: map[string]string{paramUeContextID: ue.GetSupi()},
				Body:   models.UeN1N2InfoSubscriptionCreateData{},
			})
		})
}
