// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	ctxt "context"
	"testing"
	"time"
)

// namedHandler answers with its own name, so a response shows which handler ran.
func namedHandler(name string) func(ctxt.Context, string, string, any) (any, string, any, any) {
	return func(ctxt.Context, string, string, any) (any, string, any, any) {
		return name, "", nil, nil
	}
}

// awaitQueued waits until the channel holds n messages that its goroutine has not
// taken yet.
func awaitQueued(t *testing.T, tx *EventChannel, n int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for len(tx.Message) < n {
		if time.Now().After(deadline) {
			t.Fatalf("%d message(s) queued, want %d", len(tx.Message), n)
		}
		time.Sleep(time.Millisecond)
	}
}

// Two service requests for one live UE, both queued before the UE's goroutine takes
// the first. The handler used to be a single field on the channel, set by each caller
// just before it queued its message, so by the time the goroutine ran the first
// message the second caller had replaced it -- and the first request ran the second
// request's handler.
func TestEachServiceRequestRunsItsOwnHandler(t *testing.T) {
	ue := &AmfUe{}
	ue.init()
	tx := ue.NewEventChannel()
	ue.EventChannel = tx

	type outcome struct{ requested, ran string }
	outcomes := make(chan outcome, 2)
	dispatch := func(name string) {
		go func() {
			response := ue.DispatchSbiMsg(namedHandler(name), SbiMsg{Result: make(chan SbiResponseMsg, 1)})
			ran, _ := response.RespData.(string)
			outcomes <- outcome{requested: name, ran: ran}
		}()
	}

	dispatch("ProvideLocationInfo")
	awaitQueued(t, tx, 1)
	dispatch("ProvideDomainSelectionInfo")
	awaitQueued(t, tx, 2)

	go tx.Start(ctxt.Background())
	t.Cleanup(func() { tx.Event <- "quit" })

	for range 2 {
		select {
		case o := <-outcomes:
			if o.ran != o.requested {
				t.Errorf("the %s request ran the %s handler", o.requested, o.ran)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("a queued service request was never answered")
		}
	}
}
