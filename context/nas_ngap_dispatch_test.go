// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	ctxt "context"
	"testing"
	"time"
)

// The NAS and NGAP dispatchers used to set the channel's handler field before queuing
// every message, while the channel's goroutine read that field to run the message before
// it: an unsynchronised write and read on every message for a busy UE. Each message now
// carries its handler, so a dispatcher queuing the next message touches nothing the
// goroutine is reading. Run under -race, this dispatches both kinds while the goroutine
// is working, and checks that every message ran the handler it carried.
func TestNasAndNgapMessagesRunTheHandlerTheyCarry(t *testing.T) {
	ue := &AmfUe{}
	ue.init()
	ue.SetEventChannel(ctxt.Background())
	t.Cleanup(func() { ue.EventChannel.Event <- "quit" })

	const perKind = 50
	ran := make(chan string, 2*perKind)
	nasHandler := func(_ *AmfUe, msg NasMsg) { ran <- "nas:" + string(msg.NasMsg) }
	ngapHandler := func(_ *AmfUe, msg NgapMsg) { ran <- "ngap:" + msg.Ran.GnbId }

	go func() {
		for i := range perKind {
			ue.EventChannel.SubmitMessage(NasMsg{NasMsg: []byte{byte('a' + i%26)}, Handler: nasHandler})
		}
	}()
	go func() {
		for range perKind {
			ue.EventChannel.SubmitMessage(NgapMsg{Ran: &AmfRan{GnbId: "gnb"}, Handler: ngapHandler})
		}
	}()

	counts := map[string]int{}
	for range 2 * perKind {
		select {
		case got := <-ran:
			counts[got[:4]]++
		case <-time.After(5 * time.Second):
			t.Fatalf("only %v of %d messages ran", counts, 2*perKind)
		}
	}
	if counts["nas:"] != perKind || counts["ngap"] != perKind {
		t.Fatalf("ran %v, want %d of each kind, each by its own handler", counts, perKind)
	}
}
