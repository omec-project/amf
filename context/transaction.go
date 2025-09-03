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
