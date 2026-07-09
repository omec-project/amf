// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package producer

import (
	ctxt "context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/gmm"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/fsm"
	"github.com/omec-project/util/httpwrapper"
)

type PduSession struct {
	PduSessionId string
	SmContextRef string
	Sst          string
	Sd           string
	Dnn          string
}

type UEContext struct {
	AccessType models.AccessType
	Supi       string
	Guti       string
	/* Tai */
	Mcc string
	Mnc string
	Tac string
	/* PDU sessions */
	PduSessions []PduSession
	/*Connection state */
	CmState models.CmState
}

type UEContexts []UEContext

type ActiveUeContext struct {
	AccessType models.AccessType
	Mcc        string
	Mnc        string
	Supi       string
	Guti       string
	Tmsi       string
	Tac        string

	/* RanUe Details */
	RanUeNgapId int64
	AmfUeNgapId int64

	/* Ran Details */
	GnbId string

	AmfInstanceName string
	AmfInstanceIp   string

	PduSessions []PduSession
}

type ActiveUeContexts []ActiveUeContext

func HandleOAMPurgeUEContextRequest(ctx ctxt.Context, supi, reqUri string, msg interface{}) (interface{}, string, interface{}, interface{}) {
	amfSelf := context.AMF_Self()
	if ue, ok := amfSelf.AmfUeFindBySupi(supi); ok {
		ueFsmState := ue.State[models.ACCESSTYPE__3_GPP_ACCESS].Current()
		switch ueFsmState {
		case context.Deregistered:
			logger.ProducerLog.Info("Removing the UE : ", fmt.Sprintln(ue.GetSupi()))
			ue.Remove()
		case context.Registered:
			logger.ProducerLog.Info("Deregistration triggered for the UE : ", ue.GetSupi())
			err := gmm.GmmFSM.SendEvent(ctx, ue.State[models.ACCESSTYPE__3_GPP_ACCESS], gmm.NwInitiatedDeregistrationEvent, fsm.ArgsType{
				gmm.ArgAmfUe:      ue,
				gmm.ArgAccessType: models.ACCESSTYPE__3_GPP_ACCESS,
			})
			if err != nil {
				logger.ProducerLog.Errorf("Error sending deregistration event: %v", err)
			}
		}
	}
	return nil, "", nil, nil
}

func HandleOAMRegisteredUEContext(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ProducerLog.Infof("[OAM] Handle Registered UE Context")

	supi := request.Params["supi"]

	ueContexts, problemDetails := OAMRegisteredUEContextProcedure(supi)
	if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	} else {
		return httpwrapper.NewResponse(http.StatusOK, nil, ueContexts)
	}
}

func HandleOAMActiveUEContextsFromDB(request *httpwrapper.Request) *httpwrapper.Response {
	logger.ProducerLog.Infof("[OAM] Handle Active UE Contexts Request")
	var ueContexts []ActiveUeContext
	ueList := context.DbFetchAllEntries()

	for _, ue := range ueList {
		ueContext := &ActiveUeContext{
			AccessType: models.ACCESSTYPE__3_GPP_ACCESS,
			Supi:       ue.GetSupi(),
			Guti:       ue.GetGuti(),
			Mcc:        ue.Tai.PlmnId.Mcc,
			Mnc:        ue.Tai.PlmnId.Mnc,
			Tac:        ue.Tai.Tac,
			Tmsi:       fmt.Sprintf("%08x", uint32(ue.GetTmsi())),
		}
		if ue.RanUe != nil && ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS] != nil {
			ueContext.RanUeNgapId = ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS].RanUeNgapId
			ueContext.AmfUeNgapId = ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS].AmfUeNgapId

			if ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS].Ran != nil {
				ueContext.GnbId = ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS].Ran.GnbId
			}
		}
		ueContext.AmfInstanceName = ue.AmfInstanceName
		ueContext.AmfInstanceIp = ue.AmfInstanceIp

		accessType := models.ACCESSTYPE__3_GPP_ACCESS
		ue.SmContextList.Range(func(key, value interface{}) bool {
			smContext := value.(*context.SmContext)
			if smContext.AccessType() == accessType {
				snssai := smContext.Snssai()
				pduSession := PduSession{
					PduSessionId: strconv.Itoa(int(smContext.PduSessionID())),
					SmContextRef: smContext.SmContextRef(),
					Sst:          strconv.Itoa(int(snssai.GetSst())),
					Sd:           snssai.GetSd(),
					Dnn:          smContext.Dnn(),
				}
				ueContext.PduSessions = append(ueContext.PduSessions, pduSession)
			}
			return true
		})
		ueContexts = append(ueContexts, *ueContext)
	}

	if len(ueList) == 0 {
		problemDetails := utils.ProblemDetailsContextNotFound("No UE context found")
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}

	return httpwrapper.NewResponse(http.StatusOK, nil, ueContexts)
}

func OAMRegisteredUEContextProcedure(supi string) (UEContexts, *models.ProblemDetails) {
	var ueContexts UEContexts
	amfSelf := context.AMF_Self()

	if supi != "" {
		if ue, ok := amfSelf.AmfUeFindBySupi(supi); ok {
			ueContext := buildUEContext(ue, models.ACCESSTYPE__3_GPP_ACCESS)
			if ueContext != nil {
				ueContexts = append(ueContexts, *ueContext)
			}
			ueContext = buildUEContext(ue, models.ACCESSTYPE_NON_3_GPP_ACCESS)
			if ueContext != nil {
				ueContexts = append(ueContexts, *ueContext)
			}
		} else {
			problemDetails := utils.ProblemDetailsContextNotFound("UE context not found")
			return nil, problemDetails
		}
	} else {
		amfSelf.UePool.Range(func(key, value interface{}) bool {
			ue := value.(*context.AmfUe)
			ueContext := buildUEContext(ue, models.ACCESSTYPE__3_GPP_ACCESS)
			if ueContext != nil {
				ueContexts = append(ueContexts, *ueContext)
			}
			ueContext = buildUEContext(ue, models.ACCESSTYPE_NON_3_GPP_ACCESS)
			if ueContext != nil {
				ueContexts = append(ueContexts, *ueContext)
			}
			return true
		})
	}

	return ueContexts, nil
}

func buildUEContext(ue *context.AmfUe, accessType models.AccessType) *UEContext {
	if ue.State[accessType].Is(context.Registered) {
		ueContext := &UEContext{
			AccessType: accessType,
			Supi:       ue.GetSupi(),
			Guti:       ue.GetGuti(),
			Mcc:        ue.Tai.PlmnId.Mcc,
			Mnc:        ue.Tai.PlmnId.Mnc,
			Tac:        ue.Tai.Tac,
		}

		ue.SmContextList.Range(func(key, value interface{}) bool {
			smContext := value.(*context.SmContext)
			if smContext.AccessType() == accessType {
				snssai := smContext.Snssai()
				pduSession := PduSession{
					PduSessionId: strconv.Itoa(int(smContext.PduSessionID())),
					SmContextRef: smContext.SmContextRef(),
					Sst:          strconv.Itoa(int(snssai.GetSst())),
					Sd:           snssai.GetSd(),
					Dnn:          smContext.Dnn(),
				}
				ueContext.PduSessions = append(ueContext.PduSessions, pduSession)
			}
			return true
		})

		if ue.CmConnect(accessType) {
			ueContext.CmState = models.CMSTATE_CONNECTED
		} else {
			ueContext.CmState = models.CMSTATE_IDLE
		}
		return ueContext
	}
	return nil
}
