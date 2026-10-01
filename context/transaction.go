// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"context"
	"sync"

	"github.com/omec-project/openapi/v2/utils"
)

type EventChannel struct {
	Message       chan any
	Event         chan string
	AmfUe         *AmfUe
	ConfigHandler func(ctx context.Context, s1, s2, s3 string, msg any)
	// done is closed when Start returns.
	done chan struct{}
	// admitMu is held from the removed check through the handler call it admits, so Remove's
	// TryLock (see Remove) either lands before admission and is seen by the check, or finds
	// admission already committed and leaves the running handler alone.
	admitMu sync.Mutex
}

// afterMessageDequeued runs once a message has been taken off the channel, before admitMu is
// acquired for it -- the window a test can use to drive Remove through it deterministically,
// rather than approximate it with timing. A no-op in production.
var afterMessageDequeued = func() {}

// FuncMsg is a closure submitted to a UE's EventChannel so it runs serialized with any in-flight
// NAS/NGAP message for that UE, instead of racing it from an independent goroutine (e.g. a GMM
// procedure timer's abort callback).
type FuncMsg func()

func (tx *EventChannel) UpdateConfigHandler(handler func(ctx context.Context, s1, s2, s3 string, msg any)) {
	tx.AmfUe.TxLog.Infof("updated confighandler")
	tx.ConfigHandler = handler
}

func (tx *EventChannel) Start(ctx context.Context) {
	defer close(tx.done)
	for {
		select {
		case msg := <-tx.Message:
			afterMessageDequeued()
			// Remove marks the UE before it can send quit, and select may take a queued
			// message first, so the mark is checked as each message is taken: one taken
			// after it is not run. Held across the check and the handler it admits, so
			// Remove's TryLock cannot land between the two: either it finds admitMu free
			// and marks removal before this check runs, or it finds a handler already
			// committed to and leaves it alone, like one already running when Remove is
			// called. A request is answered as for any removed UE; anything else has
			// nothing to act on. Remove is not made to wait for a running handler: most
			// removals run on this goroutine, and the rest run on an NGAP connection's
			// reader, which must not stall behind one.
			tx.admitMu.Lock()
			if tx.AmfUe.isRemoved() {
				tx.admitMu.Unlock()
				if request, isRequest := msg.(SbiMsg); isRequest {
					request.Result <- ueRemovedResponse()
				}
				continue
			}
			switch msg := msg.(type) {
			case NasMsg:
				msg.Handler(tx.AmfUe, msg)
			case NgapMsg:
				msg.Handler(tx.AmfUe, msg)
			case SbiMsg:
				p_1, p_2, p_3, p_4 := msg.Handler(ctx, msg.UeContextId, msg.ReqUri, msg.Msg)
				res := SbiResponseMsg{
					RespData:       p_1,
					LocationHeader: p_2,
					ProblemDetails: p_3,
					TransferErr:    p_4,
				}
				msg.Result <- res
			case ConfigMsg:
				tx.ConfigHandler(ctx, msg.Supi, msg.Sst, msg.Sd, msg.Msg)
			case FuncMsg:
				msg()
			}
			tx.admitMu.Unlock()
		case event := <-tx.Event:
			if event == "quit" {
				tx.AmfUe.TxLog.Infof("closed ue goroutine")
				return
			}
		}
	}
}

// SubmitMessage queues msg for the channel's goroutine, and reports false if the
// goroutine has stopped -- the UE was removed -- so that nothing is left waiting on a
// queue no one reads. The NGAP reader goroutine is one such sender, and blocking it
// would stall every UE on that gNB.
func (tx *EventChannel) SubmitMessage(msg any) bool {
	// Checked first: once the goroutine has stopped, a buffer with room is ready too, and
	// select would pick between the two at random.
	select {
	case <-tx.done:
		return false
	default:
	}
	select {
	case tx.Message <- msg:
		return true
	case <-tx.done:
		return false
	}
}

// DispatchSbiMsg runs msg on the UE's event channel and waits for the reply.
//
// A UE context restored from the datastore has no event channel: the field is json:"-"
// and DbFetch clears it. The channel is created for it here, as RunSerialized does,
// rather than running the handler on the caller's goroutine. Run there, the handler was
// serialised with nothing: NAS, NGAP or a timer could create the channel meanwhile and
// run this UE's work on it while the handler was still in progress.
func (ue *AmfUe) DispatchSbiMsg(
	handler func(ctx context.Context, s1, s2 string, msg any) (any, string, any, any),
	msg SbiMsg,
) SbiResponseMsg {
	msg.Handler = handler
	tx := ue.eventChannel()
	if !tx.SubmitMessage(msg) {
		return ueRemovedResponse()
	}
	select {
	case response := <-msg.Result:
		return response
	case <-tx.done:
		// The goroutine stopped. It sends a result before it can stop, and Result is
		// buffered, so one already produced is waiting here; otherwise the message was
		// still queued when the UE was removed and nothing will answer it.
		select {
		case response := <-msg.Result:
			return response
		default:
			return ueRemovedResponse()
		}
	}
}

// ueRemovedResponse answers a service request whose UE was removed before it was
// handled: by then there is no context to act on.
func ueRemovedResponse() SbiResponseMsg {
	return SbiResponseMsg{ProblemDetails: utils.ProblemDetailsContextNotFound("UE context removed")}
}
