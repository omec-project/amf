// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
// SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

package nas

import (
	"fmt"

	"github.com/free5gc/amf/context"
	"github.com/free5gc/amf/logger"
	"github.com/free5gc/amf/nas/nas_security"
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

		ue.AmfUe.AttachRanUe(ue)

		// set log information
		ue.AmfUe.NASLog = logger.NasLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.AmfUeNgapId))
		ue.AmfUe.GmmLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.AmfUeNgapId))

		go Start(ue.AmfUe)
	}

	var transInfo context.AmfUeTransientInfo
	transInfo.NasPdu = nasPdu
	transInfo.ProcedureCode = procedureCode
	transInfo.AnType = ue.Ran.AnType

	ue.AmfUe.TransientInfo <- transInfo
}

func DispatchMsg(amfUe *context.AmfUe, transInfo context.AmfUeTransientInfo) {

	msg, err := nas_security.Decode(amfUe, transInfo.AnType, transInfo.NasPdu)
	if err != nil {
		amfUe.NASLog.Errorln(err)
		return
	}

	if err := Dispatch(amfUe, transInfo.AnType, transInfo.ProcedureCode, msg); err != nil {
		amfUe.NASLog.Errorf("Handle NAS Error: %v", err)
	}
}

func Start(amfUe *context.AmfUe) {
	for {
		select {
		case msg := <-amfUe.TransientInfo:
			DispatchMsg(amfUe, msg)
			//		case event := <-amfUe.EventChannel:

		}
	}
}
