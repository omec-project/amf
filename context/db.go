// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"bytes"
	"fmt"
	"os"
	"sync"

	"github.com/bytedance/sonic"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/idgenerator"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var dbMutex sync.Mutex

const (
	dbWriteWorkers   = 4
	dbWriteQueueSize = 256
)

type dbWriteOp struct {
	filter bson.M
	data   bson.M
}

var (
	dbWriteCh   = make(chan dbWriteOp, dbWriteQueueSize)
	dbWriteOnce sync.Once
)

func startDBWriteWorkers() {
	dbWriteOnce.Do(func() {
		for range dbWriteWorkers {
			go func() {
				for op := range dbWriteCh {
					if _, postErr := mongoapi.CommonDBClient.RestfulAPIPost(AmfUeDataColl, op.filter, op.data); postErr != nil {
						logger.DataRepoLog.Warnln(postErr)
					}
				}
			}()
		}
	})
}

type CustomFieldsAmfUe struct {
	State       map[models.AccessType]string `json:"state"`
	SmCtxList   map[string]SmContext         `json:"smCtxList"`
	N1N2Message *N1N2Message                 `json:"n1n2Msg,omitempty"`
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
	mongoDbUrl := "mongodb://mongodb:27017"
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
			logger.DataRepoLog.Errorln("mongoDb Connection failed")
		} else {
			logger.DataRepoLog.Infoln("successfully connected to Mongodb")
			break
		}
	}
	_, err := mongoapi.CommonDBClient.CreateIndex(AmfUeDataColl, "supi")
	if err != nil {
		logger.DataRepoLog.Errorln("create index failed on Supi field")
	}

	_, err = mongoapi.CommonDBClient.CreateIndex(AmfUeDataColl, "guti")
	if err != nil {
		logger.DataRepoLog.Errorln("create index failed on Guti field")
	}

	_, err = mongoapi.CommonDBClient.CreateIndex(AmfUeDataColl, "tmsi")
	if err != nil {
		logger.DataRepoLog.Errorln("create index failed on Tmsi field")
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
	startDBWriteWorkers()
}

// amfJSONBufInitialCap is the initial capacity for pooled JSON encoding buffers
// (32 KiB covers typical UE context sizes without frequent reallocation).
const amfJSONBufInitialCap = 32 * 1024

// amfJSONBufPool pools the bytes.Buffer used for sonic encoding to reduce GC pressure.
var amfJSONBufPool = sync.Pool{
	New: func() any { return bytes.NewBuffer(make([]byte, 0, amfJSONBufInitialCap)) },
}

func ToBsonM(data *AmfUe) (ret bson.M) {
	buf := amfJSONBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	enc := sonic.ConfigDefault.NewEncoder(buf)
	if err := enc.Encode(data); err != nil {
		amfJSONBufPool.Put(buf)
		logger.DataRepoLog.Errorf("amfue marshal error: %v", err)
		return
	}
	if err := sonic.Unmarshal(buf.Bytes(), &ret); err != nil {
		logger.DataRepoLog.Errorf("amfue unmarshal error: %v", err)
	}
	amfJSONBufPool.Put(buf)
	return
}

func StoreContextInDB(ue *AmfUe) {
	self := AMF_Self()
	if !self.EnableDbStore {
		return
	}
	// Serialize synchronously (snapshot before next EventChannel message can modify ue).
	amfUeBsonA := ToBsonM(ue)
	if amfUeBsonA == nil {
		return
	}
	filter := bson.M{"supi": ue.GetSupi()}
	select {
	case dbWriteCh <- dbWriteOp{filter: filter, data: amfUeBsonA}:
	default:
		metrics.IncrementDbWriteDropped()
		logger.DataRepoLog.Warnf("DB write queue full, dropping store for supi=%s", ue.GetSupi())
	}
}

func DeleteContextFromDB(ue *AmfUe) {
	self := AMF_Self()
	if self.EnableDbStore {
		filter := bson.M{"supi": ue.GetSupi()}

		delErr := mongoapi.CommonDBClient.RestfulAPIDeleteOne(AmfUeDataColl, filter)
		if delErr != nil {
			logger.DataRepoLog.Warnln(delErr)
		}
	}
}

// dropEmptyEnumValues removes keys whose value is an empty string, in place, walking
// nested objects and arrays.
//
// Decoding an object into Go treats an absent key and an empty string identically -- the
// field is left at its zero value -- with one exception: the strict 3GPP enum decoders
// refuse an empty value, and refuse the whole document with it. A stored context that is
// only partly populated therefore becomes unreadable, and it does not take much: an
// optional container written with no class, or ngKsi.tsc on a UE stored before
// authentication finished.
//
// This runs only after a strict decode has already failed, so a well-formed record is
// never touched by it.
func dropEmptyEnumValues(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, held := range typed {
			if str, isString := held.(string); isString && str == "" {
				delete(typed, key)
				continue
			}

			dropEmptyEnumValues(held)
		}
	case []any:
		for _, held := range typed {
			dropEmptyEnumValues(held)
		}
	}
}

func DbFetch(collName string, filter bson.M) *AmfUe {
	ue := &AmfUe{}
	ue.init()
	result, getOneErr := mongoapi.CommonDBClient.RestfulAPIGetOne(collName, filter)
	if getOneErr != nil {
		logger.DataRepoLog.Warnln(getOneErr)
	}

	if len(result) == 0 {
		return nil
	}

	err := sonic.Unmarshal(mapToByte(result), ue)
	if err != nil {
		// Retry without the empty values a strict enum decoder refuses. Records written
		// before those values stopped being written are still in deployed databases,
		// and a UE whose record cannot be read is a UE that cannot be paged.
		strictErr := err

		dropEmptyEnumValues(result)

		ue = &AmfUe{}
		ue.init()

		if err = sonic.Unmarshal(mapToByte(result), ue); err != nil {
			// Not the same thing as an absent document, and conflating them is what
			// hid this for months: the context is there and unreadable, which is a
			// fault to fix rather than a subscriber to go looking for.
			logger.DataRepoLog.Errorf("stored UE context exists but could not be decoded: %v", err)

			return nil
		}

		logger.DataRepoLog.Warnf("read a stored UE context that a strict decode refused (%v); "+
			"empty values were dropped", strictErr)
	}

	dbMutex.Lock()
	defer dbMutex.Unlock()

	ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS].SetAmfUe(ue)
	AMF_Self().RanUePool.Store(ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS].AmfUeNgapId, ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS])
	AMF_Self().UePool.Store(ue.Supi, ue)
	ue.EventChannel = nil
	ue.NASLog = logger.NasLog.With(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS].AmfUeNgapId))
	ue.GmmLog = logger.GmmLog.With(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS].AmfUeNgapId))
	ue.TxLog = logger.GmmLog.With(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS].AmfUeNgapId))
	ue.ProducerLog = logger.ProducerLog.With(logger.FieldSupi, fmt.Sprintf("SUPI:%s", ue.Supi))
	ue.AmfInstanceName = os.Getenv("HOSTNAME")
	ue.AmfInstanceIp = os.Getenv("POD_IP")
	ue.TxLog.Debugln("amfue fetched")
	return ue
}

func DbFetchRanUeByRanUeNgapID(ranUeNgapID int64, ran *AmfRan) *RanUe {
	filter := bson.M{}
	filter["customFieldsAmfUe.ranUeNgapId"] = ranUeNgapID
	filter["customFieldsAmfUe.ranId"] = ran.GnbId

	ue := DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.DataRepoLog.Debugln("DbFetchRanUeByRanUeNgapID: no document found for ranUeNgapID", ranUeNgapID)
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
	return ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS]
}

func DbFetchRanUeByAmfUeNgapID(amfUeNgapID int64) *RanUe {
	self := AMF_Self()
	filter := bson.M{}
	filter["customFieldsAmfUe.amfUeNgapId"] = amfUeNgapID
	ue := DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.DataRepoLog.Errorln("DbFetchRanUeByAmfUeNgapID: no document found for amfUeNgapID ", amfUeNgapID)
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
	return ue.RanUe[models.ACCESSTYPE__3_GPP_ACCESS]
}

func DbFetchUeByGuti(guti string) (ue *AmfUe, ok bool) {
	self := AMF_Self()
	filter := bson.M{}
	filter["guti"] = guti

	ue = DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.DataRepoLog.Warnln("FindByGuti: no document found for guti", guti)
		return nil, false
	} else {
		ok = true
	}

	// Check if some parallel procedure has already
	// fetched AmfUe. If so, then return the same.
	// else return newly fetched AmfUe and store in context
	if amfUe, ret := self.AmfUeFindByGutiLocal(guti); ret {
		logger.DataRepoLog.Infoln("FindByGuti: found by local", guti)
		ue = amfUe
		ok = ret
	}

	return ue, ok
}

func DbFetchUeBySupi(supi string) (ue *AmfUe, ok bool) {
	self := AMF_Self()
	filter := bson.M{}
	filter["supi"] = supi

	ue = DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.DataRepoLog.Warnln("FindBySupi: no document found for supi", supi)
		return nil, false
	} else {
		ok = true
	}
	// Check if some parallel procedure has already
	// fetched AmfUe. If so, then return the same.
	// else return newly fetched AmfUe and store in context
	if amfUe, ret := self.AmfUeFindBySupiLocal(supi); ret {
		logger.DataRepoLog.Infoln("FindBySupi: found by local", supi)
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
		err := sonic.Unmarshal(mapToByte(val), ue)
		if err != nil {
			logger.DataRepoLog.Errorf("amfue unmarshal error: %v", err)
			return nil
		}
		ueList = append(ueList, ue)
	}

	return ueList
}
