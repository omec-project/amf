// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nas

import (
	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/nas/nas_security"
)

func HandleNAS(ue *context.RanUe, procedureCode int64, nasPdu []byte) {
	amfSelf := context.AMF_Self()

	if ue == nil {
		logger.NasLog.Error("RanUe is nil")
		return
	}

	if nasPdu == nil {
		ue.Log.Error("nasPdu is nil")
		return
	}

	if ue.AmfUe == nil {
		ue.AmfUe = nas_security.FetchUeContextWithMobileIdentity(nasPdu)
		if ue.AmfUe == nil {
			ue.AmfUe = amfSelf.NewAmfUe("")
		}

		ue.AmfUe.Mutex.Lock()
		defer ue.AmfUe.Mutex.Unlock()

		ue.AmfUe.AttachRanUe(ue)

		if ue.AmfUe.EventChannel == nil {
			ue.AmfUe.EventChannel = ue.AmfUe.NewEventChannel()
			ue.AmfUe.EventChannel.UpdateNasHandler(DispatchMsg)
			go ue.AmfUe.EventChannel.Start()
		}
		nasMsg := context.NasMsg{
			AnType:        ue.Ran.AnType,
			NasMsg:        nasPdu,
			ProcedureCode: procedureCode,
		}
		ue.AmfUe.EventChannel.SubmitMessage(nasMsg)

		return
	}

	msg, err := nas_security.Decode(ue.AmfUe, ue.Ran.AnType, nasPdu)
	if err != nil {
		ue.AmfUe.NASLog.Errorln(err)
		return
	}
	if err := Dispatch(ue.AmfUe, ue.Ran.AnType, procedureCode, msg); err != nil {
		ue.AmfUe.NASLog.Errorf("Handle NAS Error: %v", err)
	}

}

func DispatchMsg(amfUe *context.AmfUe, transInfo context.NasMsg) {

	msg, err := nas_security.Decode(amfUe, transInfo.AnType, transInfo.NasMsg)
	if err != nil {
		amfUe.NASLog.Errorln(err)
		return
	}

	if err := Dispatch(amfUe, transInfo.AnType, transInfo.ProcedureCode, msg); err != nil {
		amfUe.NASLog.Errorf("Handle NAS Error: %v", err)
	}
}
