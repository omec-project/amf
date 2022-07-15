// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"encoding/json"
	"fmt"
	"github.com/omec-project/MongoDBLibrary"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/idgenerator"
	"go.mongodb.org/mongo-driver/bson"
	"os"
	"sync"

	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/models"
)

var dbMutex sync.Mutex

type CustomFieldsAmfUe struct {
	State         map[models.AccessType]string `json:"state"`
	SmCtxList     map[string]SmContext         `json:"smCtxList"`
	N1N2Message   N1N2Message                  `json:"n1n2Msg"`
	ULCount       uint32                       `json:"ulCount"`
	DLCount       uint32                       `json:"dlCount"`
	RanUeNgapId   int64                        `json:"ranUeNgapId"`
	AmfUeNgapId   int64                        `json:"amfUeNgapId"`
	RanRemoteAddr string                       `json:"ranRemoteAddr"`
}

var Namespace = os.Getenv("POD_NAMESPACE")
var AmfUeDataColl = "amf.data.amfState"

func AllocateUniqueID(generator **idgenerator.IDGenerator, idName string) (int64, error) {
	//Use MongoDB increment field to generate new offset.
	//generate ids between offset to 8192 above offset.
	dbMutex.Lock()
	defer dbMutex.Unlock()
	if *generator == nil {
		logger.ContextLog.Infof("generator null. fetch offset from db")
		val := MongoDBLibrary.GetUniqueIdentity(idName)
		// Mongodb returns value starting from 1.
		// Limiting users to 8192(2^13) per instance.
		// TODO : Make this value configurable.
		//        Later this value can be used to trigger
		//        creation of new instance
		minVal := int64((val-1)*8192 + 1)
		maxVal := int64(minVal + 8192)
		*generator = idgenerator.NewGenerator(minVal, maxVal)
	}

	val, err := (*generator).Allocate()
	if err != nil {
		logger.ContextLog.Warnf("Max IDs generated for Instance")
		return -1, err
	}

	return val, nil
}

func SetupAmfCollection() {
	var mongoDbUrl string
	if factory.AmfConfig.Configuration.AmfDBName == "" {
		factory.AmfConfig.Configuration.AmfDBName = "sdcore_amf"
	}
	if (factory.AmfConfig.Configuration.Mongodb == nil) ||
		(factory.AmfConfig.Configuration.Mongodb.Url == "") {
		mongoDbUrl = "mongodb://mongodb:27017"
	}

	if Namespace != "" {
		AmfUeDataColl = Namespace + "." + AmfUeDataColl
	}

	MongoDBLibrary.SetMongoDB(factory.AmfConfig.Configuration.AmfDBName, mongoDbUrl)
	_, err := MongoDBLibrary.CreateIndex(AmfUeDataColl, "supi")
	if err != nil {
		logger.ContextLog.Errorf("Create index failed on Supi field.")
	}

	_, err = MongoDBLibrary.CreateIndex(AmfUeDataColl, "guti")
	if err != nil {
		logger.ContextLog.Errorf("Create index failed on Guti field.")
	}

	_, err = MongoDBLibrary.CreateIndex(AmfUeDataColl, "tmsi")
	if err != nil {
		logger.ContextLog.Errorf("Create index failed on Tmsi field.")
	}

	_, err = MongoDBLibrary.CreateIndex(AmfUeDataColl, "customFieldsAmfUe.amfUeNgapId")
	if err != nil {
		logger.ContextLog.Errorf("Create index failed on AmfUeNgapID field.")
	}

	// Indexing for ranUeNgapId would fail if we have multiple gnbs.
	// TODO: We should create index with multiple fields (ranUeNgapId & ranIpAddr)
	/*_, err = MongoDBLibrary.CreateIndex(AmfUeDataColl, "customFieldsAmfUe.ranUeNgapId")
	if err != nil {
		logger.ContextLog.Errorf("Create index failed on RanUeNgapID field.")
	}*/
}

func ToBsonM(data *AmfUe) (ret bson.M) {
	tmp, err := json.Marshal(data)
	if err != nil {
		logger.ContextLog.Errorf("amfue marshall error: %v", err)
	}
	err = json.Unmarshal(tmp, &ret)
	if err != nil {
		logger.ContextLog.Errorf("amfue unmarshall error: %v", err)
	}

	return
}

func StoreContextInDB(ue *AmfUe) {
	self := AMF_Self()
	if self.EnableDbStore {
		amfUeBsonA := ToBsonM(ue)
		filter := bson.M{"supi": ue.Supi}

		MongoDBLibrary.RestfulAPIPost(AmfUeDataColl, filter, amfUeBsonA)
	}
}

func DeleteContextFromDB(ue *AmfUe) {
	self := AMF_Self()
	if self.EnableDbStore {
		filter := bson.M{"supi": ue.Supi}

		MongoDBLibrary.RestfulAPIDeleteOne(AmfUeDataColl, filter)
	}
}

func DbFetch(collName string, filter bson.M) *AmfUe {
	ue := &AmfUe{}
	ue.init()
	result := MongoDBLibrary.RestfulAPIGetOne(collName, filter)

	if len(result) == 0 {
		logger.ContextLog.Errorf("no documents found in DB")
		return nil
	}
	err := json.Unmarshal(mapToByte(result), ue)
	if err != nil {
		logger.ContextLog.Errorf("amfue unmarshall error: %v", err)
		return nil
	}

	ue.Mutex.Lock()
	defer ue.Mutex.Unlock()

	ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUe = ue
	ue.EventChannel = nil
	ue.NASLog = logger.NasLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	ue.GmmLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	ue.TxLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	return ue
}

func DbFetchRanUeByRanUeNgapID(ranUeNgapID int64, ran *AmfRan) *RanUe {
	filter := bson.M{}
	filter["customFieldsAmfUe.ranUeNgapId"] = ranUeNgapID
	filter["customFieldsAmfUe.ranRemoteAddr"] = ran.GnbIp

	ue := DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.ContextLog.Errorf("DbFetchRanUeByRanUeNgapID: no document found for ranUeNgapID ", ranUeNgapID)
		return nil
	}

	//Check if some parallel procedure has already
	//fetched AmfUe and stored the RanUE in context.
	//If so, then return the stored RanUE
	//else return RanUE from newly fetched AmfUe
	//and store in context
	ranUe := ran.RanUeFindByRanUeNgapIDLocal(ranUeNgapID)
	if ranUe != nil {
		return ranUe
	}

	return ue.RanUe[models.AccessType__3_GPP_ACCESS]
}

func DbFetchRanUeByAmfUeNgapID(amfUeNgapID int64) *RanUe {
	self := AMF_Self()
	filter := bson.M{}
	filter["customFieldsAmfUe.amfUeNgapId"] = amfUeNgapID
	ue := DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.ContextLog.Errorf("DbFetchRanUeByAmfUeNgapID : no document found for amfUeNgapID ", amfUeNgapID)
		return nil
	}

	//Check if some parallel procedure has already
	//fetched AmfUe and stored the RanUE in context.
	//If so, then return the stored RanUE
	//else return RanUE from newly fetched AmfUe
	//and store in context
	ranUe := self.RanUeFindByAmfUeNgapIDLocal(amfUeNgapID)
	if ranUe != nil {
		return ranUe
	}

	return ue.RanUe[models.AccessType__3_GPP_ACCESS]
}

func DbFetchUeByGuti(guti string) (ue *AmfUe, ok bool) {
	self := AMF_Self()
	filter := bson.M{}
	filter["guti"] = guti

	ue = DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.ContextLog.Warnf("FindByGuti : no document found for guti ", guti)
		return nil, false
	} else {
		ok = true
	}

	//Check if some parallel procedure has already
	//fetched AmfUe. If so, then return the same.
	//else return newly fetched AmfUe and store in context
	if amfUe, ret := self.AmfUeFindByGutiLocal(guti); ret {
		logger.ContextLog.Infof("FindByGuti : found by local", guti)
		ue = amfUe
		ok = ret
	}

	return ue, ok
}
