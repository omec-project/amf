// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"testing"

	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/httpwrapper"
)

// The three N1N2 handlers queue their own message rather than going through
// DispatchSbiMsg, so each has to put the handler on it. One without it would hand the
// UE's goroutine a nil function, and that goroutine has no recover. The status request
// is driven through the channel by TestN1N2MessageTransferStatusDoesNotPanic; these
// cover the other two.

func TestN1N2TransferRunsItsProcedureThroughTheChannel(t *testing.T) {
	ue := resultUe(t, "imsi-208930100009401")

	// JsonData set, as HTTPN1N2MessageTransfer sets it before decoding the body.
	response := HandleN1N2MessageTransferRequest(&httpwrapper.Request{
		Params: map[string]string{paramUeContextID: ue.GetSupi(), paramReqURI: "/n1-n2-messages"},
		Body:   models.N1N2MessageTransferRequest{JsonData: models.NewN1N2MessageTransferReqData()},
	})

	if response == nil {
		t.Fatal("no response")
		return
	}
	// With no N1 or N2 content to send, the procedure answers with a transfer error,
	// which only it produces.
	if _, ok := response.Body.(*models.N1N2MessageTransferError); !ok {
		t.Fatalf("status %d, body %#v: want the transfer procedure's *models.N1N2MessageTransferError",
			response.Status, response.Body)
	}
}

func TestN1N2SubscribeRunsItsProcedureThroughTheChannel(t *testing.T) {
	ue := resultUe(t, "imsi-208930100009402")

	response := HandleN1N2MessageSubscirbeRequest(&httpwrapper.Request{
		Params: map[string]string{paramUeContextID: ue.GetSupi()},
		Body:   models.UeN1N2InfoSubscriptionCreateData{},
	})

	if response == nil {
		t.Fatal("no response")
		return
	}
	if response.Status != 201 {
		t.Fatalf("status = %d, want 201", response.Status)
	}
	stored := 0
	ue.N1N2MessageSubscription.Range(func(any, any) bool {
		stored++
		return true
	})
	if stored != 1 {
		t.Fatalf("%d subscription(s) stored on the UE, want the one the procedure creates", stored)
	}
}
