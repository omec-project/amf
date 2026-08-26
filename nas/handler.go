// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nas

import (
	ctxt "context"
	"os"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/nas/nas_security"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	"github.com/omec-project/openapi/v2/models"
)

func HandleNAS(ctx ctxt.Context, ue *context.RanUe, procedureCode int64, nasPdu []byte) {
	amfSelf := context.AMF_Self()

	if ue == nil {
		logger.NasLog.Errorln("RanUe is nil")
		return
	}

	if nasPdu == nil {
		ue.Log.Errorln("nasPdu is nil")
		return
	}

	// Read the association once and work from that pointer for the rest of the
	// function. Re-reading it is what makes this path fatal rather than merely wrong:
	// a release running on another goroutine clears it, and nothing here recovers from
	// a nil dereference.
	amfUe := ue.GetAmfUe()
	if amfUe == nil {
		amfUe = nas_security.FetchUeContextWithMobileIdentity(nasPdu)
		ue.SetAmfUe(amfUe)

		if amfUe == nil {
			amfUe = amfSelf.NewAmfUe("")
			ue.SetAmfUe(amfUe)
		} else {
			if amfSelf.EnableSctpLb && amfSelf.EnableDbStore {
				/* checking the guti-ue belongs to this amf instance */
				id, err := amfSelf.Drsm.FindOwnerInt32ID(amfUe.GetTmsi())
				if err != nil {
					logger.NasLog.Errorf("error checking guti-ue: %v", err)
				}
				if id != nil && id.PodName != os.Getenv("HOSTNAME") {
					rsp := &sdcoreAmfServer.AmfMessage{}
					rsp.VerboseMsg = "Redirecting Msg From AMF Pod !"
					rsp.Msgtype = sdcoreAmfServer.MsgType_REDIRECT_MSG
					rsp.AmfId = os.Getenv("HOSTNAME")
					/* TODO for this release setting pod ip to simplify logic in sctplb */
					rsp.RedirectId = id.PodIp
					rsp.GnbId = ue.Ran.GnbId
					rsp.Msg = ue.SctplbMsg
					amfUe.Remove()
					ue.Ran.Amf2RanMsgChan <- rsp
					return
				}
			}
		}

		ue.Log.Infoln("Antype from new RanUe:", ue.Ran.AnType)
		// AnType is set in SetRanId function. This is called
		// when we handle NGSetup. In case of sctplb enabled,
		// we dont call this function when AMF restarts. So we
		// need to set the AnType from stored Information.
		if amfSelf.EnableSctpLb {
			ue.Ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
		}
		amfUe.AttachRanUe(ue)

		amfUe.Mutex.Lock()
		if amfUe.EventChannel == nil {
			amfUe.EventChannel = amfUe.NewEventChannel()
			amfUe.EventChannel.UpdateNasHandler(DispatchMsg)
			go amfUe.EventChannel.Start(ctx)
		}
		amfUe.EventChannel.UpdateNasHandler(DispatchMsg)
		amfUe.Mutex.Unlock()

		nasMsg := context.NasMsg{
			Context:       ctx,
			AnType:        ue.Ran.AnType,
			NasMsg:        nasPdu,
			ProcedureCode: procedureCode,
		}
		amfUe.EventChannel.SubmitMessage(nasMsg)

		return
	}
	if amfSelf.EnableSctpLb {
		ue.Ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	}

	msg, err := nas_security.Decode(amfUe, ue.Ran.AnType, nasPdu)
	if err != nil {
		amfUe.NASLog.Errorln(err)
		return
	}
	if err := Dispatch(ctx, amfUe, ue.Ran.AnType, procedureCode, msg); err != nil {
		amfUe.NASLog.Errorf("handle NAS Error: %v", err)
	}
}

func DispatchMsg(amfUe *context.AmfUe, transInfo context.NasMsg) {
	amfUe.NASLog.Infoln("handle Nas Message")
	msg, err := nas_security.Decode(amfUe, transInfo.AnType, transInfo.NasMsg)
	if err != nil {
		amfUe.NASLog.Errorln(err)
		return
	}

	if err := Dispatch(transInfo.Context, amfUe, transInfo.AnType, transInfo.ProcedureCode, msg); err != nil {
		amfUe.NASLog.Errorf("handle NAS Error: %v", err)
	}
}
