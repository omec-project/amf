// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	ctxt "context"
	"testing"
	"time"

	"github.com/omec-project/openapi/v2/models"
)

// removedUe returns a UE whose event channel has been started and then stopped by Remove,
// waiting until its goroutine has exited.
func removedUe(t *testing.T) (*AmfUe, *EventChannel) {
	t.Helper()

	ue := &AmfUe{}
	ue.init()
	tx := ue.eventChannel()
	ue.Remove()
	select {
	case <-tx.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the event channel's goroutine did not stop after Remove")
	}

	return ue, tx
}

func answeredWithin(t *testing.T, call func() SbiResponseMsg) SbiResponseMsg {
	t.Helper()

	answered := make(chan SbiResponseMsg, 1)
	go func() { answered <- call() }()
	select {
	case response := <-answered:
		return response
	case <-time.After(5 * time.Second):
		t.Fatal("the service request was never answered")
		return SbiResponseMsg{}
	}
}

func isContextNotFound(response SbiResponseMsg) bool {
	problem, ok := response.ProblemDetails.(*models.ProblemDetails)
	return ok && problem.GetStatus() == 404
}

func neverCalled(t *testing.T) func(ctxt.Context, string, string, any) (any, string, any, any) {
	return func(ctxt.Context, string, string, any) (any, string, any, any) {
		t.Error("a handler ran for a UE that had been removed")
		return nil, "", nil, nil
	}
}

// A service request for a UE removed a moment earlier -- one that looked the UE up just
// before Remove ran -- used to be queued on a channel whose goroutine had exited, and
// wait for an answer for ever.
func TestAServiceRequestForARemovedUeIsAnswered(t *testing.T) {
	ue, _ := removedUe(t)

	response := answeredWithin(t, func() SbiResponseMsg {
		return ue.DispatchSbiMsg(neverCalled(t), SbiMsg{Result: make(chan SbiResponseMsg, 1)})
	})
	if !isContextNotFound(response) {
		t.Fatalf("answered %#v, want context not found", response)
	}
}

// A request queued when its UE is removed is answered as for a removed UE, and its
// handler does not run, whichever the goroutine takes first: the request, which it now
// refuses for a removed UE, or the quit, after which DispatchSbiMsg stops waiting.
func TestAServiceRequestQueuedWhenItsUeIsRemovedIsAnswered(t *testing.T) {
	ue := &AmfUe{}
	ue.init()
	tx := ue.eventChannel()

	busy, release := make(chan struct{}), make(chan struct{})
	ue.RunSerialized(func() {
		close(busy)
		<-release
	})
	<-busy

	answered := make(chan SbiResponseMsg, 1)
	go func() {
		answered <- ue.DispatchSbiMsg(neverCalled(t), SbiMsg{Result: make(chan SbiResponseMsg, 1)})
	}()
	awaitQueuedMessages(t, tx, 1)
	ue.Remove()
	close(release)

	select {
	case response := <-answered:
		if !isContextNotFound(response) {
			t.Fatalf("answered %#v, want context not found", response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a service request queued when its UE was removed was never answered")
	}
}

// Remove marks the UE before its quit reaches the goroutine. A request the goroutine
// takes in between -- held here by marking the UE without sending quit -- used to run its
// handler for a UE that had been removed.
func TestARequestTakenAfterItsUeIsMarkedRemovedIsNotRun(t *testing.T) {
	ue := &AmfUe{}
	ue.init()
	tx := ue.eventChannel()
	t.Cleanup(func() { tx.Event <- "quit" })

	ue.Mutex.Lock()
	ue.removed = true
	ue.Mutex.Unlock()

	response := answeredWithin(t, func() SbiResponseMsg {
		return ue.DispatchSbiMsg(neverCalled(t), SbiMsg{Result: make(chan SbiResponseMsg, 1)})
	})
	if !isContextNotFound(response) {
		t.Fatalf("answered %#v, want context not found", response)
	}
}

// Remove takes the channel's admission lock with TryLock, not Lock, so that a handler
// already running -- holding it for the call's duration -- cannot make Remove wait for it.
// The NGAP connection reader that calls Remove on a DRSM ownership mismatch depends on this:
// stalling it behind a handler would stall every UE on that gNB.
func TestRemoveDoesNotBlockBehindARunningHandler(t *testing.T) {
	ue := &AmfUe{}
	ue.init()

	busy, release := make(chan struct{}), make(chan struct{})
	ue.RunSerialized(func() {
		close(busy)
		<-release
	})
	<-busy

	done := make(chan struct{})
	go func() {
		ue.Remove()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Remove blocked behind a running handler")
	}
	close(release)
}

// TestRemoveWinningTheRaceToAdmitPreventsTheHandlerFromRunning pins the window admitMu
// closes directly, rather than relying on timing to land in it: Remove must be able to run
// to completion, including marking the UE removed, entirely inside the gap between a message
// being taken off the channel and Start acquiring admitMu to admit it. That is a narrower
// claim than TestRemoveDoesNotBlockBehindARunningHandler proves -- that one shows Remove does
// not wait for a handler already admitted, not that Remove can still win a race against one
// about to be -- and is the regression this channel exists to prevent: a handler running for
// a UE that Remove had already marked gone by the time it started.
func TestRemoveWinningTheRaceToAdmitPreventsTheHandlerFromRunning(t *testing.T) {
	ue := &AmfUe{}
	ue.init()
	ue.eventChannel()

	dequeued, proceed := make(chan struct{}), make(chan struct{})
	afterMessageDequeued = func() {
		close(dequeued)
		<-proceed
	}
	t.Cleanup(func() { afterMessageDequeued = func() {} })

	answered := make(chan SbiResponseMsg, 1)
	go func() {
		answered <- ue.DispatchSbiMsg(neverCalled(t), SbiMsg{Result: make(chan SbiResponseMsg, 1)})
	}()

	select {
	case <-dequeued:
	case <-time.After(5 * time.Second):
		t.Fatal("Start never dequeued the request")
	}

	// Remove must complete -- mark the UE removed and take, then release, admitMu -- while
	// Start is paused right here: after the message was dequeued, before admitMu is
	// acquired to admit it.
	ue.Remove()
	close(proceed)

	select {
	case response := <-answered:
		if !isContextNotFound(response) {
			t.Fatalf("answered %#v, want context not found", response)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the service request was never answered")
	}
}

// awaitQueuedMessages waits until the channel holds n messages its goroutine has not taken.
func awaitQueuedMessages(t *testing.T, tx *EventChannel, n int) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for len(tx.Message) < n {
		if time.Now().After(deadline) {
			t.Fatalf("%d message(s) queued, want %d", len(tx.Message), n)
		}
		time.Sleep(time.Millisecond)
	}
}

// A sender to a removed UE's channel used to block once the buffer filled. The NGAP
// reader goroutine is such a sender, and blocking it stalls every UE on that gNB.
func TestSubmittingToARemovedUeNeverBlocks(t *testing.T) {
	_, tx := removedUe(t)

	done := make(chan struct{})
	go func() {
		for range 2 * cap(tx.Message) {
			if tx.SubmitMessage(FuncMsg(func() {})) {
				t.Error("a message was accepted by a channel whose goroutine had stopped")
			}
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("submitting to a removed UE's channel blocked")
	}
}

// A UE restored from the datastore has no channel, so Remove had nothing to stop -- and a
// caller still holding the UE used to create a channel for it afterwards, whose goroutine
// nothing would ever stop. It is now created stopped, and the request is answered as for
// any removed UE.
func TestAChannelCreatedAfterRemovalIsStopped(t *testing.T) {
	ue := &AmfUe{}
	ue.init()
	ue.Remove()

	response := answeredWithin(t, func() SbiResponseMsg {
		return ue.DispatchSbiMsg(neverCalled(t), SbiMsg{Result: make(chan SbiResponseMsg, 1)})
	})
	if !isContextNotFound(response) {
		t.Fatalf("answered %#v, want context not found", response)
	}
	select {
	case <-ue.EventChannel.done:
	default:
		t.Fatal("a channel created after its UE was removed is running")
	}
}
