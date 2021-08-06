// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
// SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

package context

import "github.com/free5gc/openapi/models"

type Transaction struct {
	Message     chan interface{}
	Event       chan string
	AmfUe       *AmfUe
	NasHandler  func(*AmfUe, NasMsg)
	NgapHandler func(*AmfUe, NgapMsg)
	SbiHandler  func(s1, s2 string, msg interface{}) (interface{}, string, interface{}, interface{})
}

func (tx *Transaction) UpdateNgapHandler(handler func(*AmfUe, NgapMsg)) {
	tx.AmfUe.TxLog.Infof("updated ngaphandler")
	tx.NgapHandler = handler
}

func (tx *Transaction) UpdateNasHandler(handler func(*AmfUe, NasMsg)) {
	tx.AmfUe.TxLog.Infof("updated nashandler")
	tx.NasHandler = handler
}

func (tx *Transaction) UpdateSbiHandler(handler func(s1, s2 string, msg interface{}) (interface{}, string, interface{}, interface{})) {
	tx.AmfUe.TxLog.Infof("updated sbihandler")
	tx.SbiHandler = handler
}

func (tx *Transaction) Start() {
	for {
		select {
		case msg := <-tx.Message:
			switch msg.(type) {
			case NasMsg:
				tx.NasHandler(tx.AmfUe, msg.(NasMsg))
			case NgapMsg:
				tx.NgapHandler(tx.AmfUe, msg.(NgapMsg))
			case SbiMsg:
				p_1, p_2, p_3, p_4 := tx.SbiHandler(msg.(SbiMsg).UeContextId, msg.(SbiMsg).ReqUri, msg.(SbiMsg).Msg.(models.N1N2MessageTransferRequest))
				res := SbiResponseMsg{
					RespData:       p_1,
					LocationHeader: p_2,
					ProbDetails:    p_3,
					TransferErr:    p_4,
				}
				msg.(SbiMsg).Result <- res

			}
		case event := <-tx.Event:
			if event == "quit" {
				tx.AmfUe.TxLog.Infof("closed ue goroutine")
				return
			}
		}
	}
}

func (tx *Transaction) SubmitMessage(msg interface{}) {
	tx.Message <- msg
}