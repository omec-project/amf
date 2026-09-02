// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package message

import (
	"testing"
	"time"

	"github.com/omec-project/amf/context"
)

// A context read back from the database carries a whole AmfRan whose transport and logger
// did not survive storage. Sending to one must fail and return, not panic on the nil
// logger and not block for ever on the nil channel -- the caller has a pending message in
// hand and needs control back to page instead.
func TestSendToRanReturnsForARestoredRan(t *testing.T) {
	for _, sctpLb := range []bool{false, true} {
		previous := context.AMF_Self().EnableSctpLb
		context.AMF_Self().EnableSctpLb = sctpLb

		done := make(chan struct{})

		go func() {
			defer close(done)
			// No Conn, no Amf2RanMsgChan, no Log: exactly what DbFetch produces.
			SendToRan(&context.AmfRan{RanPresent: context.RanPresentGNbId}, []byte{0x00, 0x01})
		}()

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Errorf("SendToRan did not return with EnableSctpLb=%t", sctpLb)
		}

		context.AMF_Self().EnableSctpLb = previous
	}
}
