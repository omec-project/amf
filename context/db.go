// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"encoding/json"
	"fmt"
	"github.com/omec-project/MongoDBLibrary"
	"go.mongodb.org/mongo-driver/bson"

	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/models"
)

const (
	AmfUeDataColl = "amf.data.amfState"
)

func SetupAmfCollection() {
	MongoDBLibrary.SetMongoDB("sdcore", "mongodb://mongodb")
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

	_, err = MongoDBLibrary.CreateIndex(AmfUeDataColl, "amfUeNgapId")
	if err != nil {
		logger.ContextLog.Errorf("Create index failed on AmfUeNgapID field.")
	}

	_, err = MongoDBLibrary.CreateIndex(AmfUeDataColl, "ranUeNgapId")
	if err != nil {
		logger.ContextLog.Errorf("Create index failed on RanUeNgapID field.")
	}
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
		logger.ContextLog.Infof("filter : ", filter)

		MongoDBLibrary.RestfulAPIPost(AmfUeDataColl, filter, amfUeBsonA)
	}
}

func DeleteContextFromDB(ue *AmfUe) {
	self := AMF_Self()
	if self.EnableDbStore {
		filter := bson.M{"supi": ue.Supi}
		logger.ContextLog.Infof("filter : ", filter)

		MongoDBLibrary.RestfulAPIDeleteOne(AmfUeDataColl, filter)
	}
}

func DbFetchRanUeByRanUeNgapID(ranUeNgapID int64) *RanUe {
	ue := &AmfUe{}
	ue.init()
	filter := bson.M{}
	filter["ranUeNgapId"] = ranUeNgapID

	result := MongoDBLibrary.RestfulAPIGetOne(AmfUeDataColl, filter)

	logger.ContextLog.Infof("FindByRanUeNgapID, amf state json : ", result)

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
	return ue.RanUe[models.AccessType__3_GPP_ACCESS]
}

func DbFetchRanUeByAmfUeNgapID(amfUeNgapID int64) *RanUe {
	ue := &AmfUe{}
	ue.init()
	filter := bson.M{}
	filter["amfUeNgapId"] = amfUeNgapID

	result := MongoDBLibrary.RestfulAPIGetOne(AmfUeDataColl, filter)

	logger.ContextLog.Infof("FindByAmfUeNgapID, amf state json : ", result)

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
	return ue.RanUe[models.AccessType__3_GPP_ACCESS]
}

func DbFetchUeByGuti(guti string) (ue *AmfUe, ok bool) {
	ue = &AmfUe{}
	ue.init()
	filter := bson.M{}
	filter["guti"] = guti

	result := MongoDBLibrary.RestfulAPIGetOne(AmfUeDataColl, filter)

	logger.ContextLog.Infof("FindByGuti : amf state json : ", result)

	if len(result) == 0 {
		logger.ContextLog.Errorf("no documents found in DB")
		return nil, false
	}

	err := json.Unmarshal(mapToByte(result), ue)
	if err != nil {
		logger.ContextLog.Errorf("amfue unmarshall error: %v", err)
		return nil, false
	}

	ue.Mutex.Lock()
	defer ue.Mutex.Unlock()

	ue.EventChannel = nil
	ue.NASLog = logger.NasLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	ue.GmmLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	ue.TxLog = logger.GmmLog.WithField(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ue.RanUe[models.AccessType__3_GPP_ACCESS].AmfUeNgapId))
	return ue, true
}
