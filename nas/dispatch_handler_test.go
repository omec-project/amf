// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package nas

import (
	ctxt "context"
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2/models"
)

// The first NAS message on a new association is queued on the UE's event channel, and
// the message now carries the handler that runs it. Queue a closure behind it: the
// channel runs in order, so once the closure runs the NAS message has been handled. A
// message queued without its handler would have ended the process first -- the channel's
// goroutine has no recover.
func TestTheFirstNasMessageIsQueuedWithItsHandler(t *testing.T) {
	// A new association allocates a GUTI from the first served GUAMI, which a unit test
	// has to supply.
	self := context.AMF_Self()
	originalGuamis := self.ServedGuamiList
	self.ServedGuamiList = []models.Guami{{
		PlmnId: models.PlmnIdNid{Mcc: "208", Mnc: "93"},
		AmfId:  "cafe00",
	}}
	t.Cleanup(func() { self.ServedGuamiList = originalGuamis })

	ran := context.NewAmfRanDefault()
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	ranUe, err := ran.NewRanUe(9701)
	if err != nil {
		t.Fatalf("NewRanUe: %v", err)
	}
	ranUe.Log = logger.NgapLog
	t.Cleanup(func() {
		if amfUe := ranUe.GetAmfUe(); amfUe != nil && amfUe.EventChannel != nil {
			amfUe.EventChannel.Event <- "quit"
		}
		if err := ranUe.Remove(); err != nil {
			t.Logf("removing the RanUe: %v", err)
		}
	})

	// Not a decodable NAS message: DispatchMsg reports that and returns, which is all
	// this needs of it.
	HandleNAS(ctxt.Background(), ranUe, 0, []byte{0x7e, 0x00, 0x41})

	amfUe := ranUe.GetAmfUe()
	if amfUe == nil || amfUe.EventChannel == nil {
		t.Fatal("the first NAS message did not create the UE's event channel")
		return
	}
	handled := make(chan struct{})
	amfUe.RunSerialized(func() { close(handled) })
	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatal("the queued NAS message was never handled")
	}
}
