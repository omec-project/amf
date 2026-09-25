// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	ctxt "context"
	"sync"
	"testing"
	"time"
)

// A UE restored from the datastore has no event channel. Its service requests used to
// run their handler directly on the caller's goroutine -- and nothing stopped NAS, NGAP
// or a timer creating the channel meanwhile and running this UE's work on it, alongside
// the handler still in progress. The channel exists to serialise exactly that.
func TestAServiceRequestForAUeWithoutAChannelIsSerialisedWithItsOtherWork(t *testing.T) {
	ue := &AmfUe{}
	ue.init()

	entered, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	// Released here too, so a failing run does not leave the handler blocked.
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(release) })
		if ue.EventChannel != nil {
			ue.EventChannel.Event <- "quit"
		}
	})
	answered := make(chan SbiResponseMsg, 1)
	go func() {
		answered <- ue.DispatchSbiMsg(func(ctxt.Context, string, string, any) (any, string, any, any) {
			close(entered)
			<-release
			return "answered", "", nil, nil
		}, SbiMsg{Result: make(chan SbiResponseMsg, 1)})
	}()
	<-entered

	// Other work for the same UE, submitted while the service request is still running.
	ran := make(chan struct{})
	go ue.RunSerialized(func() { close(ran) })

	select {
	case <-ran:
		t.Fatal("work for this UE ran while its service request was still being handled")
	case <-time.After(100 * time.Millisecond):
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case response := <-answered:
		if response.RespData != "answered" {
			t.Fatalf("response = %#v, want the handler's", response.RespData)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the service request was never answered")
	}
	select {
	case <-ran:
	case <-time.After(5 * time.Second):
		t.Fatal("the work queued behind the service request never ran")
	}
}
