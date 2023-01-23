// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"encoding/json"
	"fmt"
	"math"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/idgenerator"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/drsm"
)

var (
	amfContext = AMFContext{}
	//tmsiGenerator                    *idgenerator.IDGenerator = nil
	//amfUeNGAPIDGenerator             *idgenerator.IDGenerator = nil
	//amfStatusSubscriptionIDGenerator *idgenerator.IDGenerator = nil
	mutex sync.Mutex
)

func init() {
	AMF_Self().LadnPool = make(map[string]*LADN)
	AMF_Self().EventSubscriptionIDGenerator = idgenerator.NewGenerator(1, math.MaxInt32)
	AMF_Self().Name = "amf"
	AMF_Self().UriScheme = models.UriScheme_HTTPS
	AMF_Self().RelativeCapacity = 0xff
	AMF_Self().ServedGuamiList = make([]models.Guami, 0, MaxNumOfServedGuamiList)
	AMF_Self().PlmnSupportList = make([]factory.PlmnSupportItem, 0, MaxNumOfPLMNs)
	AMF_Self().NfService = make(map[models.ServiceName]models.NfService)
	AMF_Self().NetworkName.Full = "free5GC"
	//tmsiGenerator = idgenerator.NewGenerator(1, math.MaxInt32)
	//amfStatusSubscriptionIDGenerator = idgenerator.NewGenerator(1, math.MaxInt32)
	//amfUeNGAPIDGenerator = idgenerator.NewGenerator(1, MaxValueOfAmfUeNgapId)
}

type AMFContext struct {
	Drsm                            drsm.DrsmInterface
	EventSubscriptionIDGenerator    *idgenerator.IDGenerator
	EventSubscriptions              sync.Map
	UePool                          sync.Map         // map[supi]*AmfUe
	RanUePool                       sync.Map         // map[AmfUeNgapID]*RanUe
	AmfRanPool                      sync.Map         // map[net.Conn]*AmfRan
	LadnPool                        map[string]*LADN // dnn as key
	SupportTaiLists                 []models.Tai
	ServedGuamiList                 []models.Guami
	PlmnSupportList                 []factory.PlmnSupportItem
	RelativeCapacity                int64
	NfId                            string
	Name                            string
	NfService                       map[models.ServiceName]models.NfService // nfservice that amf support
	UriScheme                       models.UriScheme
	BindingIPv4                     string
	SBIPort                         int
	NgapPort                        int
	SctpGrpcPort                    int
	RegisterIPv4                    string
	HttpIPv6Address                 string
	TNLWeightFactor                 int64
	SupportDnnLists                 []string
	AMFStatusSubscriptions          sync.Map // map[subscriptionID]models.SubscriptionData
	NfStatusSubscriptions           sync.Map // map[NfInstanceID]models.NrfSubscriptionData.SubscriptionId
	NrfUri                          string
	SecurityAlgorithm               SecurityAlgorithm
	NetworkName                     factory.NetworkName
	NgapIpList                      []string // NGAP Server IP
	T3502Value                      int      // unit is second
	T3512Value                      int      // unit is second
	Non3gppDeregistrationTimerValue int      // unit is second
	// read-only fields
	T3513Cfg                 factory.TimerValue
	T3522Cfg                 factory.TimerValue
	T3550Cfg                 factory.TimerValue
	T3560Cfg                 factory.TimerValue
	T3565Cfg                 factory.TimerValue
	EnableSctpLb             bool
	EnableDbStore            bool
	EnableNrfCaching         bool
	NrfCacheEvictionInterval time.Duration
}

type AMFContextEventSubscription struct {
	IsAnyUe           bool
	IsGroupUe         bool
	UeSupiList        []string
	Expiry            *time.Time
	EventSubscription models.AmfEventSubscription
}

type SecurityAlgorithm struct {
	IntegrityOrder []uint8 // slice of security.AlgIntegrityXXX
	CipheringOrder []uint8 // slice of security.AlgCipheringXXX
}

func NewPlmnSupportItem() (item factory.PlmnSupportItem) {
	item.SNssaiList = make([]models.Snssai, 0, MaxNumOfSlice)
	return
}

func (context *AMFContext) TmsiAllocate() int32 {
	//val, err := AllocateUniqueID(&tmsiGenerator, "tmsi")
	val, err := context.Drsm.AllocateInt32ID()
	if err != nil {
		logger.ContextLog.Errorf("Allocate TMSI error: %+v", err)
		return -1
	}
	logger.ContextLog.Infof("Allocate TMSI : %v", val)
	return int32(val)
}

func (context *AMFContext) AllocateAmfUeNgapID() (int64, error) {
	//val, err := AllocateUniqueID(&amfUeNGAPIDGenerator, "amfUeNgapID")
	val, err := context.Drsm.AllocateInt32ID()
	if err != nil {
		logger.ContextLog.Errorf("Allocate NgapID error: %+v", err)
		return -1, err
	}

	logger.ContextLog.Infof("Allocate AmfUeNgapID : %v", val)
	return int64(val), nil
}

func (context *AMFContext) AllocateGutiToUe(ue *AmfUe) {
	servedGuami := context.ServedGuamiList[0]
	ue.Tmsi = context.TmsiAllocate()

	plmnID := servedGuami.PlmnId.Mcc + servedGuami.PlmnId.Mnc
	tmsiStr := fmt.Sprintf("%08x", ue.Tmsi)
	ue.Guti = plmnID + servedGuami.AmfId + tmsiStr
}

func (context *AMFContext) ReAllocateGutiToUe(ue *AmfUe) {
	servedGuami := context.ServedGuamiList[0]

	context.Drsm.ReleaseInt32ID(ue.Tmsi)
	//tmsiGenerator.FreeID(int64(ue.Tmsi))

	ue.Tmsi = context.TmsiAllocate()

	plmnID := servedGuami.PlmnId.Mcc + servedGuami.PlmnId.Mnc
	tmsiStr := fmt.Sprintf("%08x", ue.Tmsi)
	ue.Guti = plmnID + servedGuami.AmfId + tmsiStr
}

func (context *AMFContext) AllocateRegistrationArea(ue *AmfUe, anType models.AccessType) {
	// clear the previous registration area if need
	if len(ue.RegistrationArea[anType]) > 0 {
		ue.RegistrationArea[anType] = nil
	}

	// allocate a new tai list as a registration area to ue
	// TODO: algorithm to choose TAI list

	taiList := make([]models.Tai, len(context.SupportTaiLists))
	copy(taiList, context.SupportTaiLists)
	for i := range taiList {
		tmp, _ := strconv.ParseUint(taiList[i].Tac, 10, 32)
		taiList[i].Tac = fmt.Sprintf("%06x", tmp)
		logger.ContextLog.Infof("Supported Tai List in AMF Plmn: %v, Tac: %v", taiList[i].PlmnId, taiList[i].Tac)
	}
	for _, supportTai := range taiList {
		if reflect.DeepEqual(supportTai, ue.Tai) {
			ue.RegistrationArea[anType] = append(ue.RegistrationArea[anType], supportTai)
			break
		}
	}
}

func (context *AMFContext) NewAMFStatusSubscription(subscriptionData models.SubscriptionData) (subscriptionID string) {
	//id, err := amfStatusSubscriptionIDGenerator.Allocate()
	id, err := context.Drsm.AllocateInt32ID()
	if err != nil {
		logger.ContextLog.Errorf("Allocate subscriptionID error: %+v", err)
		return ""
	}

	subscriptionID = strconv.Itoa(int(id))
	context.AMFStatusSubscriptions.Store(subscriptionID, subscriptionData)
	return
}

// Return Value: (subscriptionData *models.SubScriptionData, ok bool)
func (context *AMFContext) FindAMFStatusSubscription(subscriptionID string) (*models.SubscriptionData, bool) {
	if value, ok := context.AMFStatusSubscriptions.Load(subscriptionID); ok {
		subscriptionData := value.(models.SubscriptionData)
		return &subscriptionData, ok
	} else {
		return nil, false
	}
}

func (context *AMFContext) DeleteAMFStatusSubscription(subscriptionID string) {
	context.AMFStatusSubscriptions.Delete(subscriptionID)
	if id, err := strconv.ParseInt(subscriptionID, 10, 64); err != nil {
		logger.ContextLog.Error(err)
	} else {
		//amfStatusSubscriptionIDGenerator.FreeID(id)
		context.Drsm.ReleaseInt32ID(int32(id))
	}
}

func (context *AMFContext) NewEventSubscription(subscriptionID string, subscription *AMFContextEventSubscription) {
	context.EventSubscriptions.Store(subscriptionID, subscription)
}

func (context *AMFContext) FindEventSubscription(subscriptionID string) (*AMFContextEventSubscription, bool) {
	if value, ok := context.EventSubscriptions.Load(subscriptionID); ok {
		return value.(*AMFContextEventSubscription), ok
	} else {
		return nil, false
	}
}

func (context *AMFContext) DeleteEventSubscription(subscriptionID string) {
	context.EventSubscriptions.Delete(subscriptionID)
	if id, err := strconv.ParseInt(subscriptionID, 10, 32); err != nil {
		logger.ContextLog.Error(err)
	} else {
		context.EventSubscriptionIDGenerator.FreeID(id)
	}
}

func (context *AMFContext) AddAmfUeToUePool(ue *AmfUe, supi string) {
	if len(supi) == 0 {
		logger.ContextLog.Errorf("Supi is nil")
	}
	ue.Supi = supi
	context.UePool.Store(ue.Supi, ue)
}

func (context *AMFContext) NewAmfUe(supi string) *AmfUe {
	mutex.Lock()
	defer mutex.Unlock()
	ue := AmfUe{}
	ue.init()

	if supi != "" {
		context.AddAmfUeToUePool(&ue, supi)
	}

	context.AllocateGutiToUe(&ue)

	return &ue
}

func (context *AMFContext) AmfUeFindByUeContextID(ueContextID string) (*AmfUe, bool) {
	if strings.HasPrefix(ueContextID, "imsi") {
		return context.AmfUeFindBySupi(ueContextID)
	}
	if strings.HasPrefix(ueContextID, "imei") {
		return context.AmfUeFindByPei(ueContextID)
	}
	if strings.HasPrefix(ueContextID, "5g-guti") {
		guti := ueContextID[strings.LastIndex(ueContextID, "-")+1:]
		return context.AmfUeFindByGuti(guti)
	}
	return nil, false
}

func (context *AMFContext) AmfUeFindBySupi(supi string) (ue *AmfUe, ok bool) {
	if value, loadOk := context.UePool.Load(supi); loadOk {
		ue = value.(*AmfUe)
		ok = loadOk
	} else if context.EnableDbStore {
		ue, ok = DbFetchUeBySupi(supi)
		if ue != nil && ok {
			logger.ContextLog.Infof("Ue with supi found in DB : ", supi)
			context.UePool.Store(ue.Supi, ue)
		} else {
			logger.ContextLog.Infoln("Ue with Supi not found locally and in DB: ", supi)
		}
	} else {
		logger.ContextLog.Infoln("Ue with Supi not found : ", supi)
	}

	return
}

func (context *AMFContext) AmfUeFindByPei(pei string) (ue *AmfUe, ok bool) {
	context.UePool.Range(func(key, value interface{}) bool {
		candidate := value.(*AmfUe)
		if ok = (candidate.Pei == pei); ok {
			ue = candidate
			return false
		}
		return true
	})
	return
}

func (context *AMFContext) AmfUeFindBySuci(suci string) (ue *AmfUe, ok bool) {
	context.UePool.Range(func(key, value interface{}) bool {
		candidate := value.(*AmfUe)
		if ok = (candidate.Suci == suci); ok {
			ue = candidate
			return false
		}
		return true
	})
	return
}

func (context *AMFContext) AmfUeDeleteBySuci(suci string) (ue *AmfUe, ok bool) {
	context.UePool.Range(func(key, value interface{}) bool {
		candidate := value.(*AmfUe)
		if ok = (candidate.Suci == suci); ok {
			context.UePool.Delete(candidate.Supi)
			candidate.TxLog.Infof("uecontext removed based on suci")
			candidate.Remove()
			return false
		}
		return true
	})
	return
}

func (context *AMFContext) NewAmfRan(conn net.Conn) *AmfRan {
	ran := AmfRan{}
	ran.SupportedTAList = NewSupportedTAIList()
	ran.Conn = conn
	ran.GnbIp = conn.RemoteAddr().String()
	ran.Log = logger.NgapLog.WithField(logger.FieldRanAddr, conn.RemoteAddr().String())
	context.AmfRanPool.Store(conn, &ran)
	return &ran
}

// use net.Conn to find RAN context, return *AmfRan and ok bit
func (context *AMFContext) AmfRanFindByConn(conn net.Conn) (*AmfRan, bool) {
	if value, ok := context.AmfRanPool.Load(conn); ok {
		return value.(*AmfRan), ok
	}
	return nil, false
}

func (context *AMFContext) NewAmfRanAddr(remoteAddr string) *AmfRan {
	ran := AmfRan{}
	ran.SupportedTAList = NewSupportedTAIList()
	ran.GnbIp = remoteAddr
	ran.Log = logger.NgapLog.WithField(logger.FieldRanAddr, remoteAddr)
	context.AmfRanPool.Store(remoteAddr, &ran)
	return &ran
}

func (context *AMFContext) NewAmfRanId(GnbId string) *AmfRan {
	ran := AmfRan{}
	ran.SupportedTAList = NewSupportedTAIList()
	ran.GnbId = GnbId
	ran.Log = logger.NgapLog.WithField(logger.FieldRanId, GnbId)
	context.AmfRanPool.Store(GnbId, &ran)
	return &ran
}

func (context *AMFContext) AmfRanFindByGnbId(gnbId string) (*AmfRan, bool) {
	if value, ok := context.AmfRanPool.Load(gnbId); ok {
		return value.(*AmfRan), ok
	}
	return nil, false
}

// use ranNodeID to find RAN context, return *AmfRan and ok bit
func (context *AMFContext) AmfRanFindByRanID(ranNodeID models.GlobalRanNodeId) (*AmfRan, bool) {
	var ran *AmfRan
	var ok bool
	context.AmfRanPool.Range(func(key, value interface{}) bool {
		amfRan := value.(*AmfRan)
		switch amfRan.RanPresent {
		case RanPresentGNbId:
			if amfRan.RanId.GNbId.GNBValue == ranNodeID.GNbId.GNBValue {
				ran = amfRan
				ok = true
				return false
			}
		case RanPresentNgeNbId:
			if amfRan.RanId.NgeNbId == ranNodeID.NgeNbId {
				ran = amfRan
				ok = true
				return false
			}
		case RanPresentN3IwfId:
			if amfRan.RanId.N3IwfId == ranNodeID.N3IwfId {
				ran = amfRan
				ok = true
				return false
			}
		}
		return true
	})
	return ran, ok
}

func (context *AMFContext) DeleteAmfRan(conn net.Conn) {
	context.AmfRanPool.Delete(conn)
}

func (context *AMFContext) DeleteAmfRanId(gnbId string) {
	context.AmfRanPool.Delete(gnbId)
}

func (context *AMFContext) InSupportDnnList(targetDnn string) bool {
	for _, dnn := range context.SupportDnnLists {
		if dnn == targetDnn {
			return true
		}
	}
	return false
}

func (context *AMFContext) InPlmnSupportList(snssai models.Snssai) bool {
	for _, plmnSupportItem := range context.PlmnSupportList {
		for _, supportSnssai := range plmnSupportItem.SNssaiList {
			if reflect.DeepEqual(supportSnssai, snssai) {
				return true
			}
		}
	}
	return false
}

func mapToByte(data map[string]interface{}) (ret []byte) {
	ret, _ = json.Marshal(data)
	return
}

func (context *AMFContext) AmfUeFindByGutiLocal(guti string) (ue *AmfUe, ok bool) {
	context.UePool.Range(func(key, value interface{}) bool {
		candidate := value.(*AmfUe)
		if ok = (candidate.Guti == guti); ok {
			ue = candidate
			return false
		}
		return true
	})

	return
}

func (context *AMFContext) AmfUeFindBySupiLocal(supi string) (ue *AmfUe, ok bool) {
	context.UePool.Range(func(key, value interface{}) bool {
		candidate := value.(*AmfUe)
		if ok = (candidate.Supi == supi); ok {
			ue = candidate
			return false
		}
		return true
	})

	return
}

func (context *AMFContext) AmfUeFindByGuti(guti string) (ue *AmfUe, ok bool) {
	ue, ok = context.AmfUeFindByGutiLocal(guti)
	if ok {
		logger.ContextLog.Infof("Guti found locally : ", guti)
	} else if context.EnableDbStore {
		ue, ok = DbFetchUeByGuti(guti)
		if ue != nil && ok {
			logger.ContextLog.Infof("Ue with Guti found in DB : ", guti)
			context.UePool.Store(ue.Supi, ue)
		} else {
			logger.ContextLog.Infof("Ue with Guti not found locally and in DB: ", guti)
		}
	} else {
		logger.ContextLog.Infof("Ue with Guti not found : ", guti)
	}
	return
}

func (context *AMFContext) AmfUeFindByPolicyAssociationID(polAssoId string) (ue *AmfUe, ok bool) {
	context.UePool.Range(func(key, value interface{}) bool {
		candidate := value.(*AmfUe)
		if ok = (candidate.PolicyAssociationId == polAssoId); ok {
			ue = candidate
			return false
		}
		return true
	})
	return
}

func (context *AMFContext) RanUeFindByAmfUeNgapIDLocal(amfUeNgapID int64) *RanUe {
	if value, ok := context.RanUePool.Load(amfUeNgapID); ok {
		return value.(*RanUe)
	} else {
		return nil
	}
}

func (context *AMFContext) RanUeFindByAmfUeNgapID(amfUeNgapID int64) *RanUe {
	ranUe := context.RanUeFindByAmfUeNgapIDLocal(amfUeNgapID)
	if ranUe != nil {
		return ranUe
	} else {
		if context.EnableDbStore {
			ranUe = DbFetchRanUeByAmfUeNgapID(amfUeNgapID)
			if ranUe != nil {
				context.RanUePool.Store(ranUe.AmfUeNgapId, ranUe)
				return ranUe
			}
		}
	}

	logger.ContextLog.Errorf("ranUe not found with AmfUeNgapID")
	return nil
}

func (context *AMFContext) GetIPv4Uri() string {
	return fmt.Sprintf("%s://%s:%d", context.UriScheme, context.RegisterIPv4, context.SBIPort)
}

func (context *AMFContext) InitNFService(serivceName []string, version string) {
	tmpVersion := strings.Split(version, ".")
	versionUri := "v" + tmpVersion[0]
	for index, nameString := range serivceName {
		name := models.ServiceName(nameString)
		context.NfService[name] = models.NfService{
			ServiceInstanceId: strconv.Itoa(index),
			ServiceName:       name,
			Versions: &[]models.NfServiceVersion{
				{
					ApiFullVersion:  version,
					ApiVersionInUri: versionUri,
				},
			},
			Scheme:          context.UriScheme,
			NfServiceStatus: models.NfServiceStatus_REGISTERED,
			ApiPrefix:       context.GetIPv4Uri(),
			IpEndPoints: &[]models.IpEndPoint{
				{
					Ipv4Address: context.RegisterIPv4,
					Transport:   models.TransportProtocol_TCP,
					Port:        int32(context.SBIPort),
				},
			},
		}
	}
}

// Reset AMF Context
func (context *AMFContext) Reset() {
	context.AmfRanPool.Range(func(key, value interface{}) bool {
		context.UePool.Delete(key)
		return true
	})
	for key := range context.LadnPool {
		delete(context.LadnPool, key)
	}
	context.RanUePool.Range(func(key, value interface{}) bool {
		context.RanUePool.Delete(key)
		return true
	})
	context.UePool.Range(func(key, value interface{}) bool {
		context.UePool.Delete(key)
		return true
	})
	context.EventSubscriptions.Range(func(key, value interface{}) bool {
		context.DeleteEventSubscription(key.(string))
		return true
	})
	for key := range context.NfService {
		delete(context.NfService, key)
	}
	context.SupportTaiLists = context.SupportTaiLists[:0]
	context.PlmnSupportList = context.PlmnSupportList[:0]
	context.ServedGuamiList = context.ServedGuamiList[:0]
	context.RelativeCapacity = 0xff
	context.NfId = ""
	context.UriScheme = models.UriScheme_HTTPS
	context.SBIPort = 0
	context.BindingIPv4 = ""
	context.RegisterIPv4 = ""
	context.HttpIPv6Address = ""
	context.Name = "amf"
	context.NrfUri = ""
}

// Create new AMF context
func AMF_Self() *AMFContext {
	return &amfContext
}
