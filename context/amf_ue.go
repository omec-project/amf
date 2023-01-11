// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/omec-project/UeauCommon"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	"github.com/omec-project/fsm"
	"github.com/omec-project/idgenerator"
	mi "github.com/omec-project/metricfunc/pkg/metricinfo"
	"github.com/omec-project/nas/nasMessage"
	"github.com/omec-project/nas/nasType"
	"github.com/omec-project/nas/security"
	"github.com/omec-project/ngap/ngapType"
	"github.com/omec-project/openapi/models"
)

type OnGoingProcedure string

const (
	OnGoingProcedureNothing      OnGoingProcedure = "Nothing"
	OnGoingProcedurePaging       OnGoingProcedure = "Paging"
	OnGoingProcedureN2Handover   OnGoingProcedure = "N2Handover"
	OnGoingProcedureRegistration OnGoingProcedure = "Registration"
	OnGoingProcedureAbort        OnGoingProcedure = "Abort"
)

const (
	NgRanCgiPresentNRCGI    int32 = 0
	NgRanCgiPresentEUTRACGI int32 = 1
)

const (
	RecommendRanNodePresentRanNode int32 = 0
	RecommendRanNodePresentTAI     int32 = 1
)

// GMM state for UE
const (
	Deregistered            fsm.StateType = "Deregistered"
	DeregistrationInitiated fsm.StateType = "DeregistrationInitiated"
	Authentication          fsm.StateType = "Authentication"
	SecurityMode            fsm.StateType = "SecurityMode"
	ContextSetup            fsm.StateType = "ContextSetup"
	Registered              fsm.StateType = "Registered"
)

type AmfUe struct {
	//Mutex sync.Mutex `json:"mutex,omitempty" yaml:"mutex" bson:"mutex,omitempty"`
	Mutex sync.Mutex `json:"-"`
	/* the AMF which serving this AmfUe now */
	servingAMF *AMFContext `json:"servingAMF,omitempty"` // never nil

	/* Gmm State */
	State map[models.AccessType]*fsm.State `json:"-"`
	/* Registration procedure related context */
	RegistrationType5GS                uint8                           `json:"registrationType5GS,omitempty"`
	IdentityTypeUsedForRegistration    uint8                           `json:"identityTypeUsedForRegistration,omitempty"`
	RegistrationRequest                *nasMessage.RegistrationRequest `json:"registrationRequest,omitempty"`
	ServingAmfChanged                  bool                            `json:"servingAmfChanged,omitempty"`
	DeregistrationTargetAccessType     uint8                           `json:"deregistrationTargetAccessType,omitempty"` // only used when deregistration procedure is initialized by the network
	RegistrationAcceptForNon3GPPAccess []byte                          `json:"registrationAcceptForNon3GPPAccess,omitempty"`
	RetransmissionOfInitialNASMsg      bool                            `json:"retransmissionOfInitialNASMsg,omitempty"`
	/* Used for AMF relocation */
	TargetAmfProfile *models.NfProfile `json:"targetAmfProfile,omitempty"`
	TargetAmfUri     string            `json:"targetAmfUri,omitempty"`
	/* Ue Identity*/
	PlmnId              models.PlmnId `json:"plmnId,omitempty"`
	Suci                string        `json:"suci,omitempty"`
	Supi                string        `json:"supi,omitempty"`
	UnauthenticatedSupi bool          `json:"unauthenticatedSupi,omitempty"`
	Gpsi                string        `json:"gpsi,omitempty"`
	Pei                 string        `json:"pei,omitempty"`
	Tmsi                int32         `json:"tmsi,omitempty"` // 5G-Tmsi
	Guti                string        `json:"guti,omitempty"`
	GroupID             string        `json:"groupID,omitempty"`
	EBI                 int32         `json:"ebi,omitempty"`
	/* Ue Identity*/
	EventSubscriptionsInfo map[string]*AmfUeEventSubscription `json:"eventSubscriptionInfo,omitempty"`
	/* User Location*/
	RatType                  models.RatType      `json:"ratType,omitempty"`
	Location                 models.UserLocation `json:"location,omitempty"`
	Tai                      models.Tai          `json:"tai,omitempty"`
	LocationChanged          bool                `json:"locationChanged,omitempty"`
	LastVisitedRegisteredTai models.Tai          `json:"lastVisitedRegisteredTai,omitempty"`
	TimeZone                 string              `json:"timezone,omitempty"`
	/* context about udm */
	UdmId                             string                                    `json:"udmId,omitempty"`
	NudmUECMUri                       string                                    `json:"nudmUECMUri,omitempty"`
	NudmSDMUri                        string                                    `json:"nudmSDMUri,omitempty"`
	SubscriptionDataValid             bool                                      `json:"subscriptionDataValid,omitempty"`
	Reachability                      models.UeReachability                     `json:"reachability,omitempty"`
	SubscribedData                    models.SubscribedData                     `json:"subscribedData,omitempty"`
	SmfSelectionData                  *models.SmfSelectionSubscriptionData      `json:"smfSelectionData,omitempty"`
	UeContextInSmfData                *models.UeContextInSmfData                `json:"ueContextInSmfData,omitempty"`
	TraceData                         *models.TraceData                         `json:"traceData,omitempty"`
	UdmGroupId                        string                                    `json:"udmGroupId,omitempty"`
	SubscribedNssai                   []models.SubscribedSnssai                 `json:"subscribeNssai,omitempty"`
	AccessAndMobilitySubscriptionData *models.AccessAndMobilitySubscriptionData `json:"accessAndMobilitySubscriptionData,omitempty"`
	/* contex abut ausf */
	AusfGroupId                       string                      `json:"ausfGroupId,omitempty"`
	AusfId                            string                      `json:"ausfId,omitempty"`
	AusfUri                           string                      `json:"ausfUri,omitempty"`
	RoutingIndicator                  string                      `json:"routingIndicator,omitempty"`
	AuthenticationCtx                 *models.UeAuthenticationCtx `json:"authenticationCtx,omitempty"`
	AuthFailureCauseSynchFailureTimes int                         `json:"authFailureCauseSynchFailureTimes,omitempty"`
	ABBA                              []uint8                     `json:"abba,omitempty"`
	Kseaf                             string                      `json:"kseaf,omitempty"`
	Kamf                              string                      `json:"kamf,omitempty"`
	/* context about PCF */
	PcfId                        string                    `json:"pcfId,omitempty"`
	PcfUri                       string                    `json:"pcfUri,omitempty"`
	PolicyAssociationId          string                    `json:"policyAssociationId,omitempty"`
	AmPolicyUri                  string                    `json:"amPolicyUri,omitempty"`
	AmPolicyAssociation          *models.PolicyAssociation `json:"amPolicyAssociation,omitempty"`
	RequestTriggerLocationChange bool                      `json:"requestTriggerLocationChange,omitempty"` // true if AmPolicyAssociation.Trigger contains RequestTrigger_LOC_CH
	ConfigurationUpdateMessage   []byte                    `json:"configurationUpdateMessage,omitempty"`
	/* UeContextForHandover*/
	HandoverNotifyUri string `json:"handoverNotifyUri,omitempty"`
	/* N1N2Message */
	N1N2MessageIDGenerator          *idgenerator.IDGenerator `json:"n1n2MessageIDGenerator,omitempty"`
	N1N2Message                     *N1N2Message             `json:"-"`
	N1N2MessageSubscribeIDGenerator *idgenerator.IDGenerator `json:"n1n2MessageSubscribeIDGenerator,omitempty"`
	// map[int64]models.UeN1N2InfoSubscriptionCreateData; use n1n2MessageSubscriptionID as key
	N1N2MessageSubscription sync.Map `json:"n1n2MessageSubscription,omitempty"`
	/* Pdu Sesseion context */
	SmContextList sync.Map `json:"-"` // map[int32]*SmContext, pdu session id as key
	/* Related Context*/
	//RanUe map[models.AccessType]*RanUe `json:"ranUe,omitempty" yaml:"ranUe" bson:"ranUe,omitempty"`
	RanUe map[models.AccessType]*RanUe `json:"ranUe, omitEmpty"`
	/* other */
	onGoing                       map[models.AccessType]*OnGoing        `json:"onGoing,omitempty"`
	UeRadioCapability             string                                `json:"ueRadioCapability,omitempty"` // OCTET string
	Capability5GMM                nasType.Capability5GMM                `json:"capability5GMM,omitempty"`
	ConfigurationUpdateIndication nasType.ConfigurationUpdateIndication `json:"configurationUpdateIndication,omitempty"`
	/* context related to Paging */
	UeRadioCapabilityForPaging                 *UERadioCapabilityForPaging                 `json:"ueRadioCapabilityForPaging,omitempty"`
	InfoOnRecommendedCellsAndRanNodesForPaging *InfoOnRecommendedCellsAndRanNodesForPaging `json:"infoOnRecommendedCellsAndRanNodesForPaging,omitempty"`
	UESpecificDRX                              uint8                                       `json:"ueSpecificDRX,omitempty"`
	/* Security Context */
	SecurityContextAvailable bool                         `json:"securityContextAvailable,omitempty"`
	UESecurityCapability     nasType.UESecurityCapability `json:"ueSecurityCapability,omitempty"` // for security command
	NgKsi                    models.NgKsi                 `json:"ngKsi,omitempty"`
	MacFailed                bool                         `json:"macFailed,omitempty"` // set to true if the integrity check of current NAS message is failed
	KnasInt                  [16]uint8                    `json:"knasInt,omitempty"`   // 16 byte
	KnasEnc                  [16]uint8                    `json:"knasEnc,omitempty"`   // 16 byte
	Kgnb                     []uint8                      `json:"kgnb,omitempty"`      // 32 byte
	Kn3iwf                   []uint8                      `json:"kn3iwf,omitempty"`    // 32 byte
	NH                       []uint8                      `json:"nh,omitempty"`        // 32 byte
	NCC                      uint8                        `json:"ncc,omitempty"`       // 0..7
	//ULCount                  security.Count               `json:"ulCount,omitempty" yaml:"ulCount" bson:"ulCount,omitempty"`
	//DLCount                  security.Count               `json:"dlCount,omitempty" yaml:"dlCount" bson:"dlCount,omitempty"`
	ULCount      security.Count `json:"-"`
	DLCount      security.Count `json:"-"`
	CipheringAlg uint8          `json:"cipheringAlg,omitempty"`
	IntegrityAlg uint8          `json:"integrityAlg,omitempty"`
	/* Registration Area */
	RegistrationArea map[models.AccessType][]models.Tai `json:"registrationArea,omitempty"`
	LadnInfo         []LADN                             `json:"ladnInfo,omitempty"`
	/* Network Slicing related context and Nssf */
	NssfId                            string                                       `json:"nssfId,omitempty"`
	NssfUri                           string                                       `json:"nssfUri,omitempty"`
	NetworkSliceInfo                  *models.AuthorizedNetworkSliceInfo           `json:"networkSliceInfo,omitempty"`
	AllowedNssai                      map[models.AccessType][]models.AllowedSnssai `json:"allowedNssai,omitempty"`
	ConfiguredNssai                   []models.ConfiguredSnssai                    `json:"configuredNssai,omitempty"`
	NetworkSlicingSubscriptionChanged bool                                         `json:"networkSlicingSubscriptionChanged,omitempty"`
	/* T3513(Paging) */
	T3513 *Timer `json:"t3513Value,omitempty"` // for paging
	/* T3565(Notification) */
	T3565 *Timer `json:"t3565Value,omitempty"` // for NAS Notification
	/* T3560 (for authentication request/security mode command retransmission) */
	T3560 *Timer `json:"t3560Value,omitempty"`
	/* T3550 (for registration accept retransmission) */
	T3550 *Timer `json:"t3550Value,omitempty"`
	/* T3522 (for deregistration request) */
	T3522 *Timer `json:"t3522Value,omitempty"`
	/* Ue Context Release Cause */
	ReleaseCause map[models.AccessType]*CauseAll `json:"releaseCause,omitempty"`
	/* T3502 (Assigned by AMF, and used by UE to initialize registration procedure) */
	T3502Value                      int `json:"t3502Value,omitempty"`                      // Second
	T3512Value                      int `json:"t3512Value,omitempty"`                      // default 54 min
	Non3gppDeregistrationTimerValue int `json:"non3gppDeregistrationTimerValue,omitempty"` // default 54 min

	//AmfInstanceName and Ip
	AmfInstanceName string `json:"amfInstanceName,omitempty"`
	AmfInstanceIp   string `json:"amfInstanceIp,omitempty"`
	//EventChannel  chan OnGoing
	//EventChannel *EventChannel `json:"eventChannel,omitempty" yaml:"eventChannel" bson:"eventChannel,omitempty"`
	EventChannel *EventChannel `json:"-"`
	// logger
	//NASLog      *logrus.Entry `json:"nasLog,omitempty" yaml:"nasLog" bson:"nasLog,omitempty"`
	//GmmLog      *logrus.Entry `json:"gmmLog,omitempty" yaml:"gmmLog" bson:"gmmLog,omitempty"`
	//TxLog       *logrus.Entry `json:"txLog,omitempty" yaml:"txLog" bson:"txLog,omitempty"`
	//ProducerLog *logrus.Entry `json:"producerLog,omitempty" yaml:"producerLog" bson:"producerLog,omitempty"`
	NASLog      *logrus.Entry `json:"-"`
	GmmLog      *logrus.Entry `json:"-"`
	TxLog       *logrus.Entry `json:"-"`
	ProducerLog *logrus.Entry `json:"-"`
}

func (ue *AmfUe) MarshalJSON() ([]byte, error) {
	type Alias AmfUe
	stateVal := make(map[models.AccessType]string)
	smCtxListVal := make(map[string]SmContext)
	var ranUeNgapIDVal, amfUeNgapIDVal int64
	var gnbId string
	if ue.RanUe != nil && ue.RanUe[models.AccessType__3_GPP_ACCESS] != nil {
		gnbId = ue.RanUe[models.AccessType__3_GPP_ACCESS].Ran.GnbId
		if ue.RanUe[models.AccessType__3_GPP_ACCESS] != nil {
			ranUeNgapIDVal = ue.RanUe[models.AccessType__3_GPP_ACCESS].RanUeNgapId
			amfUeNgapIDVal = ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId
		}
	}

	for access, state := range ue.State {
		stateVal[access] = string(state.Current())
	}
	n1n2MsgVal := N1N2Message{}
	if ue.N1N2Message != nil {
		n1n2MsgVal = *ue.N1N2Message
		n1n2MsgVal.Request = ue.N1N2Message.Request
		n1n2MsgVal.Request.JsonData = &models.N1N2MessageTransferReqData{}
		if ue.N1N2Message.Request.JsonData != nil {
			n1n2MsgVal.Request.JsonData = ue.N1N2Message.Request.JsonData
			n1n2MsgVal.Request.JsonData.N1MessageContainer = &models.N1MessageContainer{}
			n1n2MsgVal.Request.JsonData.N2InfoContainer = &models.N2InfoContainer{}
			if ue.N1N2Message.Request.JsonData.N1MessageContainer != nil {
				*n1n2MsgVal.Request.JsonData.N1MessageContainer = *ue.N1N2Message.Request.JsonData.N1MessageContainer
			}
			if ue.N1N2Message.Request.JsonData.N2InfoContainer != nil {
				*n1n2MsgVal.Request.JsonData.N2InfoContainer = *ue.N1N2Message.Request.JsonData.N2InfoContainer
			}
		}

	}

	ue.SmContextList.Range(func(key, val interface{}) bool {
		smContext := val.(*SmContext)
		pduSessId := smContext.PduSessionID()
		newSmCtx := NewSmContext(pduSessId)
		newSmCtx.SetPduSessionID(smContext.PduSessionID())
		newSmCtx.SetSmContextRef(smContext.SmContextRef())
		newSmCtx.SetSmfUri(smContext.SmfUri())
		newSmCtx.SetSmfID(smContext.SmfID())
		newSmCtx.SetSnssai(smContext.Snssai())
		newSmCtx.SetDnn(smContext.Dnn())
		newSmCtx.SetAccessType(smContext.AccessType())
		newSmCtx.SetHSmfID(smContext.HSmfID())
		newSmCtx.SetVSmfID(smContext.VSmfID())
		newSmCtx.SetNsInstance(smContext.NsInstance())

		pduSessIdStr := strconv.FormatInt(int64(pduSessId), 10)
		smCtxListVal[pduSessIdStr] = *newSmCtx
		return true
	})

	customAmfUe := CustomFieldsAmfUe{
		State:       stateVal,
		SmCtxList:   smCtxListVal,
		ULCount:     ue.ULCount.Get(),
		DLCount:     ue.DLCount.Get(),
		RanUeNgapId: ranUeNgapIDVal,
		AmfUeNgapId: amfUeNgapIDVal,
		N1N2Message: n1n2MsgVal,
		RanId:       gnbId,
	}

	return json.Marshal(&struct {
		CustomAmfUe CustomFieldsAmfUe `json:"customFieldsAmfUe"`
		*Alias
	}{
		CustomAmfUe: customAmfUe,
		Alias:       (*Alias)(ue),
	})
}

func (ue *AmfUe) UnmarshalJSON(data []byte) error {
	type Alias AmfUe
	auxCustom := &struct {
		CustomAmfUe CustomFieldsAmfUe `json:"customFieldsAmfUe"`
		*Alias
	}{
		Alias: (*Alias)(ue),
	}
	if err := json.Unmarshal(data, &auxCustom); err != nil {
		logger.ContextLog.Errorln("AMFUe Unmarshal failed : ", err)
		return err
	}

	aux := auxCustom.CustomAmfUe
	ran, ok := AMF_Self().AmfRanFindByGnbId(aux.RanId)
	if !ok {
		logger.ContextLog.Warnln("Ran Connection is not Exist with GnbID: ", aux.RanId)
	}
	for index, states := range aux.State {
		ue.State[index] = fsm.NewState(fsm.StateType(states))
		if ue.RanUe[index] == nil {
			ue.RanUe[index] = &RanUe{}
		}
		ue.RanUe[index].RanUeNgapId = aux.RanUeNgapId
		ue.RanUe[index].AmfUeNgapId = aux.AmfUeNgapId
		ue.RanUe[index].Log = logger.NgapLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[index].AmfUeNgapId))
		if ran != nil {
			//ran.RanUeList = append(ran.RanUeList, ue.RanUe[index])
			ue.RanUe[index].Ran = ran

		}
	}
	for key, val := range aux.SmCtxList {
		keyVal, _ := strconv.ParseInt(key, 10, 32)
		ue.StoreSmContext(int32(keyVal), &val)
	}
	sqn := uint8(aux.ULCount & 0x000000ff)
	overflow := uint16((aux.ULCount & 0x00ffff00) >> 8)
	ue.ULCount.Set(overflow, sqn)
	sqn = uint8(aux.DLCount & 0x000000ff)
	overflow = uint16((aux.DLCount & 0x00ffff00) >> 8)
	ue.DLCount.Set(overflow, sqn)
	ue.N1N2Message = &aux.N1N2Message
	return nil
}

type InterfaceType uint8

const (
	NgapMessage InterfaceType = iota
	SbiMessage
	NasMessage
)

type InterfaceMsg interface {
}

/*type InterfaceMsg struct {
	AnType        models.AccessType
	NasMsg        []byte
	ProcedureCode int64
	NgapMsg       *ngapType.NGAPPDU
	Ran           *AmfRan
	//MsgType is Nas or Sbi interface msg
	IntfType InterfaceType
}*/

type NasMsg struct {
	AnType        models.AccessType
	NasMsg        []byte
	ProcedureCode int64
}

type NgapMsg struct {
	SctplbMsg *sdcoreAmfServer.SctplbMessage
	NgapMsg   *ngapType.NGAPPDU
	Ran       *AmfRan
}

type SbiResponseMsg struct {
	RespData       interface{}
	LocationHeader string
	ProblemDetails interface{}
	TransferErr    interface{}
}

type SbiMsg struct {
	Msg         interface{}
	UeContextId string
	ReqUri      string

	Result chan SbiResponseMsg
}

type ConfigMsg struct {
	Msg  interface{}
	Supi string
	Sst  string
	Sd   string
}

type AmfUeEventSubscription struct {
	Timestamp         time.Time
	AnyUe             bool
	RemainReports     *int32
	EventSubscription *models.AmfEventSubscription
}

type N1N2Message struct {
	Request     models.N1N2MessageTransferRequest
	Status      models.N1N2MessageTransferCause
	ResourceUri string
}

type OnGoing struct {
	Procedure OnGoingProcedure
	Ppi       int32 // Paging priority
}

type UERadioCapabilityForPaging struct {
	NR    string // OCTET string
	EUTRA string // OCTET string
}

// TS 38.413 9.3.1.100
type InfoOnRecommendedCellsAndRanNodesForPaging struct {
	RecommendedCells    []RecommendedCell  // RecommendedCellsForPaging
	RecommendedRanNodes []RecommendRanNode // RecommendedRanNodesForPaging
}

// TS 38.413 9.3.1.71
type RecommendedCell struct {
	NgRanCGI         NGRANCGI
	TimeStayedInCell *int64
}

// TS 38.413 9.3.1.101
type RecommendRanNode struct {
	Present         int32
	GlobalRanNodeId *models.GlobalRanNodeId
	Tai             *models.Tai
}

type NGRANCGI struct {
	Present  int32
	NRCGI    *models.Ncgi
	EUTRACGI *models.Ecgi
}

func (ue *AmfUe) init() {
	ue.servingAMF = AMF_Self()
	ue.State = make(map[models.AccessType]*fsm.State)
	ue.State[models.AccessType__3_GPP_ACCESS] = fsm.NewState(Deregistered)
	ue.State[models.AccessType_NON_3_GPP_ACCESS] = fsm.NewState(Deregistered)
	ue.UnauthenticatedSupi = true
	ue.EventSubscriptionsInfo = make(map[string]*AmfUeEventSubscription)
	ue.RanUe = make(map[models.AccessType]*RanUe)
	ue.RegistrationArea = make(map[models.AccessType][]models.Tai)
	ue.AllowedNssai = make(map[models.AccessType][]models.AllowedSnssai)
	ue.N1N2MessageIDGenerator = idgenerator.NewGenerator(1, 2147483647)
	ue.N1N2MessageSubscribeIDGenerator = idgenerator.NewGenerator(1, 2147483647)
	ue.onGoing = make(map[models.AccessType]*OnGoing)
	ue.onGoing[models.AccessType_NON_3_GPP_ACCESS] = new(OnGoing)
	ue.onGoing[models.AccessType_NON_3_GPP_ACCESS].Procedure = OnGoingProcedureNothing
	ue.onGoing[models.AccessType__3_GPP_ACCESS] = new(OnGoing)
	ue.onGoing[models.AccessType__3_GPP_ACCESS].Procedure = OnGoingProcedureNothing
	ue.ReleaseCause = make(map[models.AccessType]*CauseAll)
	ue.AmfInstanceName = os.Getenv("HOSTNAME")
	ue.AmfInstanceIp = os.Getenv("POD_IP")
	//ue.TransientInfo = make(chan AmfUeTransientInfo, 10)
}

func (ue *AmfUe) ServingAMF() *AMFContext {
	return ue.servingAMF
}

func (ue *AmfUe) CmConnect(anType models.AccessType) bool {
	if _, ok := ue.RanUe[anType]; !ok {
		return false
	}
	return true
}

func (ue *AmfUe) CmIdle(anType models.AccessType) bool {
	return !ue.CmConnect(anType)
}

func (ue *AmfUe) Remove() {
	for _, ranUe := range ue.RanUe {
		if err := ranUe.Remove(); err != nil {
			logger.ContextLog.Errorf("Remove RanUe error: %v", err)
		}
	}

	//tmsiGenerator.FreeID(int64(ue.Tmsi))
	AMF_Self().Drsm.ReleaseInt32ID(ue.Tmsi)

	if len(ue.Supi) > 0 {
		AMF_Self().UePool.Delete(ue.Supi)
	}
	if ue.EventChannel != nil {
		ue.EventChannel.Event <- "quit"
	}
}

func (ue *AmfUe) DetachRanUe(anType models.AccessType) {
	delete(ue.RanUe, anType)
}

func (ue *AmfUe) AttachRanUe(ranUe *RanUe) {
	/* detach any RanUe associated to it */
	oldRanUe := ue.RanUe[ranUe.Ran.AnType]
	if oldRanUe != nil {
		oldRanUe.Log.Infof("Detached UeContext from OldRanUe")
		ue.RanUe[ranUe.Ran.AnType].AmfUe = nil
	}
	ue.RanUe[ranUe.Ran.AnType] = ranUe
	ranUe.AmfUe = ue

	// set log information
	ue.NASLog = logger.NasLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ranUe.AmfUeNgapId))
	ue.GmmLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ranUe.AmfUeNgapId))
	ue.TxLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ranUe.AmfUeNgapId))
}

func (ue *AmfUe) GetAnType() models.AccessType {
	if ue.CmConnect(models.AccessType__3_GPP_ACCESS) {
		return models.AccessType__3_GPP_ACCESS
	} else if ue.CmConnect(models.AccessType_NON_3_GPP_ACCESS) {
		return models.AccessType_NON_3_GPP_ACCESS
	}
	return ""
}

func (ue *AmfUe) GetCmInfo() (cmInfos []models.CmInfo) {
	var cmInfo models.CmInfo
	cmInfo.AccessType = models.AccessType__3_GPP_ACCESS
	if ue.CmConnect(cmInfo.AccessType) {
		cmInfo.CmState = models.CmState_CONNECTED
	} else {
		cmInfo.CmState = models.CmState_IDLE
	}
	cmInfos = append(cmInfos, cmInfo)
	cmInfo.AccessType = models.AccessType_NON_3_GPP_ACCESS
	if ue.CmConnect(cmInfo.AccessType) {
		cmInfo.CmState = models.CmState_CONNECTED
	} else {
		cmInfo.CmState = models.CmState_IDLE
	}
	cmInfos = append(cmInfos, cmInfo)
	return
}

func (ue *AmfUe) InAllowedNssai(targetSNssai models.Snssai, anType models.AccessType) bool {
	for _, allowedSnssai := range ue.AllowedNssai[anType] {
		if reflect.DeepEqual(*allowedSnssai.AllowedSnssai, targetSNssai) {
			return true
		}
	}
	return false
}

func (ue *AmfUe) InSubscribedNssai(targetSNssai models.Snssai) bool {
	for _, sNssai := range ue.SubscribedNssai {
		if reflect.DeepEqual(*sNssai.SubscribedSnssai, targetSNssai) {
			return true
		}
	}
	return false
}

func (ue *AmfUe) GetNsiInformationFromSnssai(anType models.AccessType, snssai models.Snssai) *models.NsiInformation {
	for _, allowedSnssai := range ue.AllowedNssai[anType] {
		if reflect.DeepEqual(*allowedSnssai.AllowedSnssai, snssai) {
			// TODO: select NsiInformation based on operator policy
			if len(allowedSnssai.NsiInformationList) != 0 {
				return &allowedSnssai.NsiInformationList[0]
			}
		}
	}
	return nil
}

func (ue *AmfUe) TaiListInRegistrationArea(taiList []models.Tai, accessType models.AccessType) bool {
	for _, tai := range taiList {
		if !InTaiList(tai, ue.RegistrationArea[accessType]) {
			return false
		}
	}
	return true
}

func (ue *AmfUe) HasWildCardSubscribedDNN() bool {
	for _, snssaiInfo := range ue.SmfSelectionData.SubscribedSnssaiInfos {
		for _, dnnInfo := range snssaiInfo.DnnInfos {
			if dnnInfo.Dnn == "*" {
				return true
			}
		}
	}
	return false
}

func (ue *AmfUe) SecurityContextIsValid() bool {
	return ue.SecurityContextAvailable && ue.NgKsi.Ksi != nasMessage.NasKeySetIdentifierNoKeyIsAvailable && !ue.MacFailed
}

// Kamf Derivation function defined in TS 33.501 Annex A.7
func (ue *AmfUe) DerivateKamf() {
	supiRegexp, err := regexp.Compile("(?:imsi|supi)-([0-9]{5,15})")
	if err != nil {
		logger.ContextLog.Error(err)
		return
	}
	groups := supiRegexp.FindStringSubmatch(ue.Supi)
	if groups == nil {
		logger.NasLog.Errorln("supi is not correct")
		return
	}

	P0 := []byte(groups[1])
	L0 := UeauCommon.KDFLen(P0)
	P1 := ue.ABBA
	L1 := UeauCommon.KDFLen(P1)

	KseafDecode, err := hex.DecodeString(ue.Kseaf)
	if err != nil {
		logger.ContextLog.Error(err)
		return
	}
	KamfBytes := UeauCommon.GetKDFValue(KseafDecode, UeauCommon.FC_FOR_KAMF_DERIVATION, P0, L0, P1, L1)
	ue.Kamf = hex.EncodeToString(KamfBytes)
}

// Algorithm key Derivation function defined in TS 33.501 Annex A.9
func (ue *AmfUe) DerivateAlgKey() {
	// Security Key
	P0 := []byte{security.NNASEncAlg}
	L0 := UeauCommon.KDFLen(P0)
	P1 := []byte{ue.CipheringAlg}
	L1 := UeauCommon.KDFLen(P1)

	KamfBytes, err := hex.DecodeString(ue.Kamf)
	if err != nil {
		logger.ContextLog.Error(err)
		return
	}
	kenc := UeauCommon.GetKDFValue(KamfBytes, UeauCommon.FC_FOR_ALGORITHM_KEY_DERIVATION, P0, L0, P1, L1)
	copy(ue.KnasEnc[:], kenc[16:32])

	// Integrity Key
	P0 = []byte{security.NNASIntAlg}
	L0 = UeauCommon.KDFLen(P0)
	P1 = []byte{ue.IntegrityAlg}
	L1 = UeauCommon.KDFLen(P1)

	kint := UeauCommon.GetKDFValue(KamfBytes, UeauCommon.FC_FOR_ALGORITHM_KEY_DERIVATION, P0, L0, P1, L1)
	copy(ue.KnasInt[:], kint[16:32])
}

// Access Network key Derivation function defined in TS 33.501 Annex A.9
func (ue *AmfUe) DerivateAnKey(anType models.AccessType) {
	accessType := security.AccessType3GPP // Defalut 3gpp
	P0 := make([]byte, 4)
	binary.BigEndian.PutUint32(P0, ue.ULCount.Get())
	L0 := UeauCommon.KDFLen(P0)
	if anType == models.AccessType_NON_3_GPP_ACCESS {
		accessType = security.AccessTypeNon3GPP
	}
	P1 := []byte{accessType}
	L1 := UeauCommon.KDFLen(P1)

	KamfBytes, err := hex.DecodeString(ue.Kamf)
	if err != nil {
		logger.ContextLog.Error(err)
		return
	}
	key := UeauCommon.GetKDFValue(KamfBytes, UeauCommon.FC_FOR_KGNB_KN3IWF_DERIVATION, P0, L0, P1, L1)
	switch accessType {
	case security.AccessType3GPP:
		ue.Kgnb = key
	case security.AccessTypeNon3GPP:
		ue.Kn3iwf = key
	}
}

// NH Derivation function defined in TS 33.501 Annex A.10
func (ue *AmfUe) DerivateNH(syncInput []byte) {
	P0 := syncInput
	L0 := UeauCommon.KDFLen(P0)

	KamfBytes, err := hex.DecodeString(ue.Kamf)
	if err != nil {
		logger.ContextLog.Error(err)
		return
	}
	ue.NH = UeauCommon.GetKDFValue(KamfBytes, UeauCommon.FC_FOR_NH_DERIVATION, P0, L0)
}

func (ue *AmfUe) UpdateSecurityContext(anType models.AccessType) {
	ue.DerivateAnKey(anType)
	switch anType {
	case models.AccessType__3_GPP_ACCESS:
		ue.DerivateNH(ue.Kgnb)
	case models.AccessType_NON_3_GPP_ACCESS:
		ue.DerivateNH(ue.Kn3iwf)
	}
	ue.NCC = 1
}

func (ue *AmfUe) UpdateNH() {
	ue.NCC++
	ue.DerivateNH(ue.NH)
}

func (ue *AmfUe) SelectSecurityAlg(intOrder, encOrder []uint8) {
	ue.CipheringAlg = security.AlgCiphering128NEA0
	ue.IntegrityAlg = security.AlgIntegrity128NIA0

	ueSupported := uint8(0)
	for _, intAlg := range intOrder {
		switch intAlg {
		case security.AlgIntegrity128NIA0:
			ueSupported = ue.UESecurityCapability.GetIA0_5G()
		case security.AlgIntegrity128NIA1:
			ueSupported = ue.UESecurityCapability.GetIA1_128_5G()
		case security.AlgIntegrity128NIA2:
			ueSupported = ue.UESecurityCapability.GetIA2_128_5G()
		case security.AlgIntegrity128NIA3:
			ueSupported = ue.UESecurityCapability.GetIA3_128_5G()
		}
		if ueSupported == 1 {
			ue.IntegrityAlg = intAlg
			break
		}
	}

	ueSupported = uint8(0)
	for _, encAlg := range encOrder {
		switch encAlg {
		case security.AlgCiphering128NEA0:
			ueSupported = ue.UESecurityCapability.GetEA0_5G()
		case security.AlgCiphering128NEA1:
			ueSupported = ue.UESecurityCapability.GetEA1_128_5G()
		case security.AlgCiphering128NEA2:
			ueSupported = ue.UESecurityCapability.GetEA2_128_5G()
		case security.AlgCiphering128NEA3:
			ueSupported = ue.UESecurityCapability.GetEA3_128_5G()
		}
		if ueSupported == 1 {
			ue.CipheringAlg = encAlg
			break
		}
	}
}

// this is clearing the transient data of registration request, this is called entrypoint of Deregistration and Registration state
func (ue *AmfUe) ClearRegistrationRequestData(accessType models.AccessType) {
	ue.RegistrationRequest = nil
	ue.RegistrationType5GS = 0
	ue.IdentityTypeUsedForRegistration = 0
	ue.AuthFailureCauseSynchFailureTimes = 0
	ue.ServingAmfChanged = false
	ue.RegistrationAcceptForNon3GPPAccess = nil
	if ue.RanUe != nil && ue.RanUe[accessType] != nil {
		ue.RanUe[accessType].UeContextRequest = false
		ue.RanUe[accessType].RecvdInitialContextSetupResponse = false
	}
	ue.RetransmissionOfInitialNASMsg = false
	ue.onGoing[accessType].Procedure = OnGoingProcedureNothing
}

// this method called when we are reusing the same uecontext during the registration procedure
func (ue *AmfUe) ClearRegistrationData() {
	//Allowed Nssai should be cleared first as it is a new Registration
	ue.SubscribedNssai = nil
	ue.AllowedNssai = make(map[models.AccessType][]models.AllowedSnssai)
	ue.SubscriptionDataValid = false
	//Clearing SMContextList locally
	ue.SmContextList.Range(func(key, _ interface{}) bool {
		ue.SmContextList.Delete(key)
		return true
	})

}

func (ue *AmfUe) SetOnGoing(anType models.AccessType, onGoing *OnGoing) {
	prevOnGoing := ue.onGoing[anType]
	ue.onGoing[anType] = onGoing
	ue.GmmLog.Debugf("OnGoing[%s]->[%s] PPI[%d]->[%d]", prevOnGoing.Procedure, onGoing.Procedure,
		prevOnGoing.Ppi, onGoing.Ppi)
}

func (ue *AmfUe) OnGoing(anType models.AccessType) OnGoing {
	return *ue.onGoing[anType]
}

func (ue *AmfUe) RemoveAmPolicyAssociation() {
	ue.AmPolicyAssociation = nil
	ue.PolicyAssociationId = ""
}

func (ue *AmfUe) CopyDataFromUeContextModel(ueContext models.UeContext) {
	if ueContext.Supi != "" {
		ue.Supi = ueContext.Supi
		ue.UnauthenticatedSupi = ueContext.SupiUnauthInd
	}

	if ueContext.Pei != "" {
		ue.Pei = ueContext.Pei
	}

	if ueContext.UdmGroupId != "" {
		ue.UdmGroupId = ueContext.UdmGroupId
	}

	if ueContext.AusfGroupId != "" {
		ue.AusfGroupId = ueContext.AusfGroupId
	}

	if ueContext.RoutingIndicator != "" {
		ue.RoutingIndicator = ueContext.RoutingIndicator
	}

	if ueContext.SubUeAmbr != nil {
		if ue.AccessAndMobilitySubscriptionData == nil {
			ue.AccessAndMobilitySubscriptionData = new(models.AccessAndMobilitySubscriptionData)
		}
		if ue.AccessAndMobilitySubscriptionData.SubscribedUeAmbr == nil {
			ue.AccessAndMobilitySubscriptionData.SubscribedUeAmbr = new(models.AmbrRm)
		}

		subAmbr := ue.AccessAndMobilitySubscriptionData.SubscribedUeAmbr
		subAmbr.Uplink = ueContext.SubUeAmbr.Uplink
		subAmbr.Downlink = ueContext.SubUeAmbr.Downlink
	}

	if ueContext.SubRfsp != 0 {
		if ue.AccessAndMobilitySubscriptionData == nil {
			ue.AccessAndMobilitySubscriptionData = new(models.AccessAndMobilitySubscriptionData)
		}
		ue.AccessAndMobilitySubscriptionData.RfspIndex = ueContext.SubRfsp
	}

	if len(ueContext.RestrictedRatList) > 0 {
		if ue.AccessAndMobilitySubscriptionData == nil {
			ue.AccessAndMobilitySubscriptionData = new(models.AccessAndMobilitySubscriptionData)
		}
		ue.AccessAndMobilitySubscriptionData.RatRestrictions = ueContext.RestrictedRatList
	}

	if len(ueContext.ForbiddenAreaList) > 0 {
		if ue.AccessAndMobilitySubscriptionData == nil {
			ue.AccessAndMobilitySubscriptionData = new(models.AccessAndMobilitySubscriptionData)
		}
		ue.AccessAndMobilitySubscriptionData.ForbiddenAreas = ueContext.ForbiddenAreaList
	}

	if ueContext.ServiceAreaRestriction != nil {
		if ue.AccessAndMobilitySubscriptionData == nil {
			ue.AccessAndMobilitySubscriptionData = new(models.AccessAndMobilitySubscriptionData)
		}
		ue.AccessAndMobilitySubscriptionData.ServiceAreaRestriction = ueContext.ServiceAreaRestriction
	}

	if ueContext.SeafData != nil {
		seafData := ueContext.SeafData

		ue.NgKsi = *seafData.NgKsi
		if seafData.KeyAmf != nil {
			if seafData.KeyAmf.KeyType == models.KeyAmfType_KAMF {
				ue.Kamf = seafData.KeyAmf.KeyVal
			}
		}
		if nh, err := hex.DecodeString(seafData.Nh); err != nil {
			logger.ContextLog.Error(err)
			return
		} else {
			ue.NH = nh
		}
		ue.NCC = uint8(seafData.Ncc)
	}

	if ueContext.PcfId != "" {
		ue.PcfId = ueContext.PcfId
	}

	if ueContext.PcfAmPolicyUri != "" {
		ue.AmPolicyUri = ueContext.PcfAmPolicyUri
	}

	if len(ueContext.AmPolicyReqTriggerList) > 0 {
		if ue.AmPolicyAssociation == nil {
			ue.AmPolicyAssociation = new(models.PolicyAssociation)
		}
		for _, trigger := range ueContext.AmPolicyReqTriggerList {
			switch trigger {
			case models.AmPolicyReqTrigger_LOCATION_CHANGE:
				ue.AmPolicyAssociation.Triggers = append(ue.AmPolicyAssociation.Triggers, models.RequestTrigger_LOC_CH)
			case models.AmPolicyReqTrigger_PRA_CHANGE:
				ue.AmPolicyAssociation.Triggers = append(ue.AmPolicyAssociation.Triggers, models.RequestTrigger_PRA_CH)
			case models.AmPolicyReqTrigger_SARI_CHANGE:
				ue.AmPolicyAssociation.Triggers = append(ue.AmPolicyAssociation.Triggers, models.RequestTrigger_SERV_AREA_CH)
			case models.AmPolicyReqTrigger_RFSP_INDEX_CHANGE:
				ue.AmPolicyAssociation.Triggers = append(ue.AmPolicyAssociation.Triggers, models.RequestTrigger_RFSP_CH)
			}
		}
	}

	if len(ueContext.SessionContextList) > 0 {
		for _, pduSessionContext := range ueContext.SessionContextList {
			smContext := SmContext{
				PduSessionIDVal: pduSessionContext.PduSessionId,
				SmContextRefVal: pduSessionContext.SmContextRef,
				SnssaiVal:       *pduSessionContext.SNssai,
				DnnVal:          pduSessionContext.Dnn,
				AccessTypeVal:   pduSessionContext.AccessType,
				HSmfIDVal:       pduSessionContext.HsmfId,
				VSmfIDVal:       pduSessionContext.VsmfId,
				NsInstanceVal:   pduSessionContext.NsInstance,
			}
			ue.StoreSmContext(pduSessionContext.PduSessionId, &smContext)
		}
	}

	if len(ueContext.MmContextList) > 0 {
		for _, mmContext := range ueContext.MmContextList {
			if mmContext.AccessType == models.AccessType__3_GPP_ACCESS {
				if nasSecurityMode := mmContext.NasSecurityMode; nasSecurityMode != nil {
					switch nasSecurityMode.IntegrityAlgorithm {
					case models.IntegrityAlgorithm_NIA0:
						ue.IntegrityAlg = security.AlgIntegrity128NIA0
					case models.IntegrityAlgorithm_NIA1:
						ue.IntegrityAlg = security.AlgIntegrity128NIA1
					case models.IntegrityAlgorithm_NIA2:
						ue.IntegrityAlg = security.AlgIntegrity128NIA2
					case models.IntegrityAlgorithm_NIA3:
						ue.IntegrityAlg = security.AlgIntegrity128NIA3
					}

					switch nasSecurityMode.CipheringAlgorithm {
					case models.CipheringAlgorithm_NEA0:
						ue.CipheringAlg = security.AlgCiphering128NEA0
					case models.CipheringAlgorithm_NEA1:
						ue.CipheringAlg = security.AlgCiphering128NEA1
					case models.CipheringAlgorithm_NEA2:
						ue.CipheringAlg = security.AlgCiphering128NEA2
					case models.CipheringAlgorithm_NEA3:
						ue.CipheringAlg = security.AlgCiphering128NEA3
					}

					if mmContext.NasDownlinkCount != 0 {
						overflow := uint16((uint32(mmContext.NasDownlinkCount) & 0x00ffff00) >> 8)
						sqn := uint8(uint32(mmContext.NasDownlinkCount & 0x000000ff))
						ue.DLCount.Set(overflow, sqn)
					}

					if mmContext.NasUplinkCount != 0 {
						overflow := uint16((uint32(mmContext.NasUplinkCount) & 0x00ffff00) >> 8)
						sqn := uint8(uint32(mmContext.NasUplinkCount & 0x000000ff))
						ue.ULCount.Set(overflow, sqn)
					}

					// TS 29.518 Table 6.1.6.3.2.1
					if mmContext.UeSecurityCapability != "" {
						// ue.SecurityCapabilities
						buf, err := base64.StdEncoding.DecodeString(mmContext.UeSecurityCapability)
						if err != nil {
							logger.ContextLog.Error(err)
							return
						}
						ue.UESecurityCapability.Buffer = buf
						ue.UESecurityCapability.SetLen(uint8(len(buf)))
					}
				}
			}

			if mmContext.AllowedNssai != nil {
				for _, snssai := range mmContext.AllowedNssai {
					allowedSnssai := models.AllowedSnssai{
						AllowedSnssai: &snssai,
					}
					ue.AllowedNssai[mmContext.AccessType] = append(ue.AllowedNssai[mmContext.AccessType], allowedSnssai)
				}
			}
		}
	}
	if ueContext.TraceData != nil {
		ue.TraceData = ueContext.TraceData
	}
}

// SM Context realted function

func (ue *AmfUe) StoreSmContext(pduSessionID int32, smContext *SmContext) {
	ue.SmContextList.Store(pduSessionID, smContext)
}

func (ue *AmfUe) SmContextFindByPDUSessionID(pduSessionID int32) (*SmContext, bool) {
	if value, ok := ue.SmContextList.Load(pduSessionID); ok {
		return value.(*SmContext), true
	} else {
		return nil, false
	}
}

func (ue *AmfUe) SetEventChannel(handler func(*AmfUe, NgapMsg)) {
	ue.Mutex.Lock()
	defer ue.Mutex.Unlock()
	if ue.EventChannel == nil {
		ue.TxLog.Errorf("Creating new AmfUe EventChannel")
		ue.EventChannel = ue.NewEventChannel()
		ue.EventChannel.AmfUe = ue
		ue.EventChannel.UpdateNgapHandler(handler)
		go ue.EventChannel.Start()
	}

	return
}

func (ue *AmfUe) NewEventChannel() (tx *EventChannel) {
	ue.TxLog.Infof("New EventChannel created")
	tx = &EventChannel{
		Message: make(chan interface{}, 10),
		Event:   make(chan string, 10),
		AmfUe:   ue,
	}
	//tx.Message <- msg
	return tx
}

func getPublishUeCtxtInfoOp(state fsm.StateType) mi.SubscriberOp {

	switch state {
	case Deregistered:
		return mi.SubsOpDel
	case DeregistrationInitiated:
		return mi.SubsOpDel
	case Authentication:
		return mi.SubsOpAdd
	case SecurityMode:
		return mi.SubsOpMod
	case ContextSetup:
		return mi.SubsOpMod
	case Registered:
		return mi.SubsOpMod
	default:
		return mi.SubsOpMod
	}
}

//Collect Ctxt info and publish on Kafka stream
func (ueContext *AmfUe) PublishUeCtxtInfo() {
	op := getPublishUeCtxtInfoOp(ueContext.State[models.AccessType__3_GPP_ACCESS].Current())
	kafkaSmCtxt := mi.CoreSubscriber{}

	//Populate kafka sm ctxt struct
	kafkaSmCtxt.Imsi = ueContext.Supi
	kafkaSmCtxt.AmfId = ueContext.servingAMF.NfId
	kafkaSmCtxt.Guti = ueContext.Guti
	kafkaSmCtxt.Tmsi = ueContext.Tmsi
	kafkaSmCtxt.AmfIp = ueContext.AmfInstanceIp
	if ueContext.RanUe != nil && ueContext.RanUe[models.AccessType__3_GPP_ACCESS] != nil {
		kafkaSmCtxt.AmfNgapId = ueContext.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId
		kafkaSmCtxt.RanNgapId = ueContext.RanUe[models.AccessType__3_GPP_ACCESS].RanUeNgapId
		kafkaSmCtxt.GnbId = ueContext.RanUe[models.AccessType__3_GPP_ACCESS].Ran.GnbId
		kafkaSmCtxt.TacId = ueContext.RanUe[models.AccessType__3_GPP_ACCESS].Tai.Tac
	}
	kafkaSmCtxt.AmfSubState = string(ueContext.State[models.AccessType__3_GPP_ACCESS].Current())
	ueState := ueContext.GetCmInfo()
	kafkaSmCtxt.UeState = string(ueState[0].CmState)

	//Send to stream
	metrics.GetWriter().PublishUeCtxtEvent(kafkaSmCtxt, op)
}
