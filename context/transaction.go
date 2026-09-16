// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"context"
)

type EventChannel struct {
	Message       chan any
	Event         chan string
	AmfUe         *AmfUe
	NasHandler    func(*AmfUe, NasMsg)
	NgapHandler   func(*AmfUe, NgapMsg)
	SbiHandler    func(ctx context.Context, s1, s2 string, msg any) (any, string, any, any)
	ConfigHandler func(ctx context.Context, s1, s2, s3 string, msg any)
}

// FuncMsg is a closure submitted to a UE's EventChannel so it runs serialized with any in-flight
// NAS/NGAP message for that UE, instead of racing it from an independent goroutine (e.g. a GMM
// procedure timer's abort callback).
type FuncMsg func()

func (tx *EventChannel) UpdateNgapHandler(handler func(*AmfUe, NgapMsg)) {
	tx.AmfUe.TxLog.Infof("updated ngaphandler")
	tx.NgapHandler = handler
}

func (tx *EventChannel) UpdateNasHandler(handler func(*AmfUe, NasMsg)) {
	tx.AmfUe.TxLog.Infof("updated nashandler")
	tx.NasHandler = handler
}

func (tx *EventChannel) UpdateSbiHandler(handler func(ctx context.Context, s1, s2 string, msg any) (any, string, any, any)) {
	tx.AmfUe.TxLog.Infof("updated sbihandler")
	tx.SbiHandler = handler
}

func (tx *EventChannel) UpdateConfigHandler(handler func(ctx context.Context, s1, s2, s3 string, msg any)) {
	tx.AmfUe.TxLog.Infof("updated confighandler")
	tx.ConfigHandler = handler
}

func (tx *EventChannel) Start(ctx context.Context) {
	for {
		select {
		case msg := <-tx.Message:
			switch msg := msg.(type) {
			case NasMsg:
				tx.NasHandler(tx.AmfUe, msg)
			case NgapMsg:
				tx.NgapHandler(tx.AmfUe, msg)
			case SbiMsg:
				p_1, p_2, p_3, p_4 := tx.SbiHandler(ctx, msg.UeContextId, msg.ReqUri, msg.Msg)
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
		case event := <-tx.Event:
			if event == "quit" {
				tx.AmfUe.TxLog.Infof("closed ue goroutine")
				return
			}
		}
	}
}

func (tx *EventChannel) SubmitMessage(msg any) {
	tx.Message <- msg
}

// DispatchSbiMsg hands msg to the UE's event channel and waits for the reply.
//
// A UE context restored from the datastore has no event channel: the field is
// json:"-" and DbFetch clears it (context/db.go), while only the NAS and NGAP
// dispatchers ever create one. A service handler that dispatches through the
// channel without asking whether it is there therefore dereferences nil for
// every request naming such a UE, which ginRecover turns into a 500 -- so the
// UE is unreachable over the service interface until something re-creates the
// channel. For that UE the handler is run on the caller's goroutine instead.
//
// The direct call uses a background context because the channel path never
// hands the handler a request-scoped one: EventChannel.Start is given the
// dispatcher's context and passes that to every SBI message it will ever run.
// Cancelling a UE-mutating procedure half way through because the client hung
// up is not the behaviour this is restoring.
func (ue *AmfUe) DispatchSbiMsg(
	handler func(ctx context.Context, s1, s2 string, msg any) (any, string, any, any),
	msg SbiMsg,
) SbiResponseMsg {
	// Mutex is the lock SetEventChannel and the NAS dispatcher take to create the
	// channel, so the read is taken under it -- and released before the call,
	// because the handlers reach StoreContextInDB and MarshalJSON takes it too.
	ue.Mutex.Lock()
	tx := ue.EventChannel
	ue.Mutex.Unlock()

	if tx != nil {
		tx.UpdateSbiHandler(handler)
		tx.SubmitMessage(msg)
		return <-msg.Result
	}

	ue.TxLog.Warnln("no event channel for this UE; running the service handler directly")
	respData, locationHeader, problemDetails, transferErr := handler(
		context.Background(), msg.UeContextId, msg.ReqUri, msg.Msg)
	return SbiResponseMsg{
		RespData:       respData,
		LocationHeader: locationHeader,
		ProblemDetails: problemDetails,
		TransferErr:    transferErr,
	}
}
