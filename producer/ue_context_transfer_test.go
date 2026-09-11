// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
)

// A peer AMF completing a UE context transfer names the sessions this AMF should release. It
// can name one this AMF holds no SM context for - the UE may never have had it, or it may have
// been released already - and the release consumer reads the SMF's URI out of that context
// before it builds anything. Since this procedure runs on the UE's event-channel goroutine,
// which has no recover(), handing it the missing context ended the process rather than the
// request, taking every other UE with it.
func TestTransferredReleaseSkipsASessionWithNoSmContext(t *testing.T) {
	self := context.AMF_Self()

	ue := self.NewAmfUe("imsi-208930100007531")
	ue.SetGuti("20893cafe0000531")
	t.Cleanup(ue.Remove)

	// Reaching the assertions at all is the point: without the guard this panics.
	rsp, problemDetails := registrationStatusUpdateProcedure(ctxt.Background(), "5g-guti-20893cafe0000531",
		models.UeRegStatusUpdateReqData{
			TransferStatus:       models.UECONTEXTTRANSFERSTATUS_TRANSFERRED,
			ToReleaseSessionList: []int32{1},
		})

	if problemDetails != nil {
		t.Fatalf("registrationStatusUpdateProcedure() problem details = %+v, want none", problemDetails)
	}

	if rsp == nil || !rsp.RegStatusTransferComplete {
		t.Errorf("registrationStatusUpdateProcedure() = %+v, want the transfer reported complete", rsp)
	}
}
