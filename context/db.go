// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"

	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/util/idgenerator"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/bson"
)

var dbMutex sync.Mutex

type UeStateForDB struct {
	AmfUeBsonA  []byte
	Supi        string
	AmfUeNgapID int64
	RanUeNgapID int64
	GnbId       string
	Guti        string
}

var StoreAmContextDbChannel chan UeStateForDB
var DeleteAmContextDbChannel chan UeStateForDB

type CustomFieldsAmfUe struct {
	State       map[models.AccessType]string `json:"state"`
	SmCtxList   map[string]SmContext         `json:"smCtxList"`
	N1N2Message N1N2Message                  `json:"n1n2Msg"`
	ULCount     uint32                       `json:"ulCount"`
	DLCount     uint32                       `json:"dlCount"`
	RanUeNgapId int64                        `json:"ranUeNgapId"`
	AmfUeNgapId int64                        `json:"amfUeNgapId"`
	RanId       string                       `json:"ranId"`
}

var (
	Namespace     = os.Getenv("POD_NAMESPACE")
	AmfUeDataColl = "amf.data.amfState"
)

func AllocateUniqueID(generator **idgenerator.IDGenerator, idName string) (int64, error) {
	// Use MongoDB increment field to generate new offset.
	// generate ids between offset to 8192 above offset.
	dbMutex.Lock()
	defer dbMutex.Unlock()
	if *generator == nil {
		logger.DataRepoLog.Infof("generator null. fetch offset from db")
		val := mongoapi.CommonDBClient.GetUniqueIdentity(idName)
		// Mongodb returns value starting from 1.
		// Limiting users to 8192(2^13) per instance.
		// TODO : Make this value configurable.
		//        Later this value can be used to trigger
		//        creation of new instance
		minVal := int64((val-1)*8192 + 1)
		maxVal := minVal + 8192
		*generator = idgenerator.NewGenerator(minVal, maxVal)
	}

	val, err := (*generator).Allocate()
	if err != nil {
		logger.DataRepoLog.Warnf("Max IDs generated for Instance")
		return -1, err
	}

	return val, nil
}

func SetupAmfCollection() {
	var mongoDbUrl string = "mongodb://mongodb:27017"
	if factory.AmfConfig.Configuration.AmfDBName == "" {
		factory.AmfConfig.Configuration.AmfDBName = "sdcore_amf"
	}

	if (factory.AmfConfig.Configuration.Mongodb != nil) &&
		(factory.AmfConfig.Configuration.Mongodb.Url != "") {
		mongoDbUrl = factory.AmfConfig.Configuration.Mongodb.Url
	}

	logger.DataRepoLog.Infof("MondbName: %v, Url: %v", factory.AmfConfig.Configuration.AmfDBName, mongoDbUrl)

	if Namespace != "" {
		AmfUeDataColl = Namespace + "." + AmfUeDataColl
	}
	for {
		mongoapi.ConnectMongo(mongoDbUrl, factory.AmfConfig.Configuration.AmfDBName)
		if mongoapi.CommonDBClient.(*mongoapi.MongoClient).Client == nil {
			logger.DataRepoLog.Errorf("MongoDb Connection failed")
		} else {
			logger.DataRepoLog.Infof("Successfully connected to Mongodb")
			break
		}
	}
	_, err := mongoapi.CommonDBClient.CreateIndex(AmfUeDataColl, "supi")
	if err != nil {
		logger.DataRepoLog.Errorf("Create index failed on Supi field.")
	}

	_, err = mongoapi.CommonDBClient.CreateIndex(AmfUeDataColl, "guti")
	if err != nil {
		logger.DataRepoLog.Errorf("Create index failed on Guti field.")
	}

	_, err = mongoapi.CommonDBClient.CreateIndex(AmfUeDataColl, "tmsi")
	if err != nil {
		logger.DataRepoLog.Errorf("Create index failed on Tmsi field.")
	}

	/*_, err = CommonDBClient.CreateIndex(AmfUeDataColl, "customFieldsAmfUe.amfUeNgapId")
	if err != nil {
		logger.DataRepoLog.Errorf("Create index failed on AmfUeNgapID field.")
	}*/

	// Indexing for ranUeNgapId would fail if we have multiple gnbs.
	// TODO: We should create index with multiple fields (ranUeNgapId & ranIpAddr)
	/*_, err = CommonDBClient.CreateIndex(AmfUeDataColl, "customFieldsAmfUe.ranUeNgapId")
	if err != nil {
		logger.DataRepoLog.Errorf("Create index failed on RanUeNgapID field.")
	}*/

	StoreAmContextDbChannel = make(chan UeStateForDB, 8192)
	DeleteAmContextDbChannel = make(chan UeStateForDB, 8192)

	// StoreAmContextDbChannel = make(chan *AmfUe, 8192)
	// DeleteAmContextDbChannel = make(chan *AmfUe, 8192)

	for i := 1; i < 256; i++ {
		go ProcessStoreContextInDB()
		go ProcessDeleteContextFromDB()
	}

}

func ToBsonM(data *AmfUe) (ret bson.M) {
	tmp, err := json.Marshal(data)
	if err != nil {
		logger.DataRepoLog.Errorf("amfue marshall error: %v", err)
	}
	err = json.Unmarshal(tmp, &ret)
	if err != nil {
		logger.DataRepoLog.Errorf("amfue unmarshall error: %v", err)
	}

	return
}

func ProcessStoreContextInDB() {
	for {
		rcvdAmfUE := <-StoreAmContextDbChannel
		amfJsom := rcvdAmfUE.AmfUeBsonA
		err := AMF_Self().RedisClient.Set(rcvdAmfUE.Supi, amfJsom, 0).Err()
		if err != nil {
			logger.ContextLog.Warnln("Error in storing in redis: ", err, "supi: ", rcvdAmfUE.Supi)
		}
		if rcvdAmfUE.AmfUeNgapID != 0 {
			amfUeNgapIDstr := strconv.FormatInt(rcvdAmfUE.AmfUeNgapID, 10)
			err = AMF_Self().RedisClient.Set(amfUeNgapIDstr, amfJsom, 0).Err()
			if err != nil {
				logger.ContextLog.Warnln("Error in storing in redis: ", err, "amfUeNgapIDstr: ", amfUeNgapIDstr)
			}
		}

		if rcvdAmfUE.RanUeNgapID != 0 {
			ranUeNgapIDValstr := strconv.FormatInt(rcvdAmfUE.RanUeNgapID, 10)
			err = AMF_Self().RedisClient.Set(ranUeNgapIDValstr, amfJsom, 0).Err()
			if err != nil {
				logger.ContextLog.Warnln("Error in storing in redis: ", err, "ranUeNgapIDValstr: ", ranUeNgapIDValstr)
			}
		}

		if rcvdAmfUE.GnbId != "" {
			err = AMF_Self().RedisClient.Set(rcvdAmfUE.GnbId, amfJsom, 0).Err()
			if err != nil {
				logger.ContextLog.Warnln("Error in storing in redis: ", err, "gnbId: ", rcvdAmfUE.GnbId)
			}
		}

		err = AMF_Self().RedisClient.Set(rcvdAmfUE.Guti, amfJsom, 0).Err()
		if err != nil {
			logger.ContextLog.Warnln("Error in storing in redis: ", err, "guti: ", rcvdAmfUE.Guti)
		}

		logger.ContextLog.Warnln("Stored: Db ue Supi: ", rcvdAmfUE.Supi, "ue.Guti", rcvdAmfUE.Guti, "ranUeNgapIDVal: ", rcvdAmfUE.RanUeNgapID, "amfUeNgapIDVal: ", rcvdAmfUE.AmfUeNgapID, "gnbId: ", rcvdAmfUE.GnbId)
	}
}

func StoreContextInDB(ue *AmfUe) {
	self := AMF_Self()
	if self.EnableDbStore {
		amfJson, err := json.Marshal(ue)
		if err != nil {
			logger.ContextLog.Errorf("amfue marshall error: %v", err)
		} else {
			var ranUeNgapIDVal, amfUeNgapIDVal int64
			var gnbId string
			ue.RanUeLock.RLock()
			if ue.RanUe != nil {
				ranUe := ue.RanUe[models.AccessType__3_GPP_ACCESS]
				if ranUe != nil {
					gnbId = ranUe.Ran.GnbId
					ranUeNgapIDVal = ranUe.RanUeNgapId
					amfUeNgapIDVal = ranUe.AmfUeNgapId
				}
			}
			ue.RanUeLock.RUnlock()
			ueState := UeStateForDB{amfJson, ue.Supi, amfUeNgapIDVal, ranUeNgapIDVal, gnbId, ue.Guti}
			StoreAmContextDbChannel <- ueState
		}
	}
}

func ProcessDeleteContextFromDB() {
	for {
		rcvdAmfUE := <-DeleteAmContextDbChannel
		_, err := AMF_Self().RedisClient.Del(rcvdAmfUE.Supi).Result()
		if err != nil {
			logger.ContextLog.Warnln("Error in deleting from redis: ", err, "supi: ", rcvdAmfUE.Supi)
		}
	}
}

func DeleteContextFromDB(ue *AmfUe) {
	self := AMF_Self()
	if self.EnableDbStore {
		ueState := UeStateForDB{nil, ue.Supi, 0, 0, "", ue.Guti}
		DeleteAmContextDbChannel <- ueState
	}
}

func DbFetch(key string) *AmfUe {
	ue := &AmfUe{}
	ue.init()

	result, err := AMF_Self().RedisClient.Get(key).Result()
	if err != nil {
		logger.ContextLog.Warnln("Error in fetching from redis: ", err, "key: ", key)
		return nil
	}
	logger.ContextLog.Infoln("Redis DB result: ", result)
	// err = json.Unmarshal(mapToByte(result), ue)
	err = json.Unmarshal([]byte(result), ue)
	if err != nil {
		logger.ContextLog.Errorf("amfue unmarshall error: %v", err)
		return nil
	}

	// dbMutex.Lock() // should it bere here? check Bilal
	// defer dbMutex.Unlock()
	ue.RanUeLock.Lock()
	defer ue.RanUeLock.Unlock()
	ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUe = ue
	AMF_Self().RanUePool.Store(ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId, ue.RanUe[models.AccessType__3_GPP_ACCESS])
	AMF_Self().UePool.Store(ue.Supi, ue)
	ue.EventChannel = nil
	ue.NASLog = logger.NasLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	ue.GmmLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	ue.TxLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	ue.ProducerLog = logger.ProducerLog.WithField(logger.FieldSupi, fmt.Sprintf("SUPI:%s", ue.Supi))
	ue.AmfInstanceName = os.Getenv("HOSTNAME")
	ue.AmfInstanceIp = os.Getenv("POD_IP")
	ue.TxLog.Errorln("amfue fetched")
	return ue
}

func DbFetchMongoDb(collName string, filter bson.M) *AmfUe {
	ue := &AmfUe{}
	ue.init()
	result, getOneErr := mongoapi.CommonDBClient.RestfulAPIGetOne(collName, filter)
	if getOneErr != nil {
		logger.DataRepoLog.Warnln(getOneErr)
	}

	if len(result) == 0 {
		return nil
	}
	err := json.Unmarshal(mapToByte(result), ue)
	if err != nil {
		logger.DataRepoLog.Errorf("amfue unmarshall error: %v", err)
		return nil
	}

	dbMutex.Lock()
	defer dbMutex.Unlock()
	ue.RanUeLock.Lock()
	defer ue.RanUeLock.Unlock()
	ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUe = ue
	AMF_Self().RanUePool.Store(ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId, ue.RanUe[models.AccessType__3_GPP_ACCESS])
	AMF_Self().UePool.Store(ue.Supi, ue)
	ue.EventChannel = nil
	ue.NASLog = logger.NasLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	ue.GmmLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	ue.TxLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	ue.ProducerLog = logger.ProducerLog.WithField(logger.FieldSupi, fmt.Sprintf("SUPI:%s", ue.Supi))
	ue.AmfInstanceName = os.Getenv("HOSTNAME")
	ue.AmfInstanceIp = os.Getenv("POD_IP")
	ue.TxLog.Errorln("amfue fetched")
	return ue
}

func DbFetchRanUeByRanUeNgapID(ranUeNgapID int64, ran *AmfRan) *RanUe {
	ranUeNgapIDstr := strconv.FormatInt(ranUeNgapID, 10)
	ue := DbFetch(ranUeNgapIDstr)
	if ue == nil {
		logger.DataRepoLog.Errorln("DbFetchRanUeByRanUeNgapID: no document found for ranUeNgapID ", ranUeNgapID)
		return nil
	}

	// Check if some parallel procedure has already
	// fetched AmfUe and stored the RanUE in context.
	// If so, then return the stored RanUE
	// else return RanUE from newly fetched AmfUe
	// and store in context
	ranUe := ran.RanUeFindByRanUeNgapIDLocal(ranUeNgapID)
	if ranUe != nil {
		return ranUe
	}
	ue.RanUeLock.RLock()
	ranUe = ue.RanUe[models.AccessType__3_GPP_ACCESS] // TODO - Bilal Change since start
	ran.RanUeList = append(ran.RanUeList, ranUe)
	ue.RanUeLock.RUnlock()
	return ranUe
}

func DbFetchRanUeByAmfUeNgapID(amfUeNgapID int64) *RanUe {
	self := AMF_Self()
	amfUeNgapIDstr := strconv.FormatInt(amfUeNgapID, 10)
	ue := DbFetch(amfUeNgapIDstr)
	if ue == nil {
		logger.DataRepoLog.Errorln("DbFetchRanUeByAmfUeNgapID : no document found for amfUeNgapID ", amfUeNgapID)
		return nil
	}

	// Check if some parallel procedure has already
	// fetched AmfUe and stored the RanUE in context.
	// If so, then return the stored RanUE
	// else return RanUE from newly fetched AmfUe
	// and store in context
	ranUe := self.RanUeFindByAmfUeNgapIDLocal(amfUeNgapID)
	if ranUe != nil {
		return ranUe
	}
	ue.RanUeLock.RLock()
	ranUe = ue.RanUe[models.AccessType__3_GPP_ACCESS]
	ue.RanUeLock.RUnlock()
	return ranUe
}

func DbFetchUeByGuti(guti string) (ue *AmfUe, ok bool) {
	self := AMF_Self()

	ue = DbFetch(guti)
	if ue == nil {
		logger.DataRepoLog.Warnln("FindByGuti : no document found for guti ", guti)
		return nil, false
	} else {
		ok = true
	}

	// Check if some parallel procedure has already
	// fetched AmfUe. If so, then return the same.
	// else return newly fetched AmfUe and store in context
	if amfUe, ret := self.AmfUeFindByGutiLocal(guti); ret {
		logger.DataRepoLog.Infoln("FindByGuti : found by local", guti)
		ue = amfUe
		ok = ret
	}

	return ue, ok
}

func DbFetchUeBySupi(supi string) (ue *AmfUe, ok bool) {

	self := AMF_Self()
	ue = DbFetch(supi)
	if ue == nil {
		logger.DataRepoLog.Warnln("FindBySupi : no document found for supi ", supi)
		return nil, false
	} else {
		ok = true
	}
	// Check if some parallel procedure has already
	// fetched AmfUe. If so, then return the same.
	// else return newly fetched AmfUe and store in context
	if amfUe, ret := self.AmfUeFindBySupiLocal(supi); ret {
		logger.DataRepoLog.Infoln("FindBySupi : found by local", supi)
		ue = amfUe
		ok = ret
	}

	return ue, ok
}

func DbFetchAllEntries() (ueList []*AmfUe) {
	ue := &AmfUe{}
	filter := bson.M{}
	results, getManyErr := mongoapi.CommonDBClient.RestfulAPIGetMany(AmfUeDataColl, filter)
	if getManyErr != nil {
		logger.DataRepoLog.Warnln(getManyErr)
	}

	for _, val := range results {
		ue = &AmfUe{}
		ue.init()
		err := json.Unmarshal(mapToByte(val), ue)
		if err != nil {
			logger.DataRepoLog.Errorf("amfue unmarshall error: %v", err)
			return nil
		}
		ueList = append(ueList, ue)
	}

	return ueList
}
