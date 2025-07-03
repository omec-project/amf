// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
package drsm

import (
	"context"
	"time"

	"github.com/omec-project/util/logger"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type UpdatedFields struct {
	ExpireAt    time.Time `bson:"expireAt,omitempty"`
	PodId       string    `bson:"podId,omitempty"`
	PodIp       string    `bson:"podIp,omitempty"`
	PodInstance string    `bson:"podInstance,omitempty"`
}

type UpdatedDesc struct {
	UpdFields UpdatedFields `bson:"updatedFields,omitempty"`
}

type FullStream struct {
	Id          string    `bson:"_id"`
	ChunkId     string    `bson:"chunkId"`
	PodId       string    `bson:"podId,omitempty"`
	PodIp       string    `bson:"podIp,omitempty"`
	PodInstance string    `bson:"podInstance,omitempty"`
	ExpireAt    time.Time `bson:"expireAt,omitempty"`
	Type        string    `bson:"type,omitempty"`
}

type DocKey struct {
	Id string `bson:"_id,omitempty"`
}

type streamDoc struct {
	DId    DocKey      `bson:"documentKey,omitempty"`
	OpType string      `bson:"operationType,omitempty"`
	Full   FullStream  `bson:"fullDocument,omitempty"`
	Update UpdatedDesc `bson:"updateDescription,omitempty"`
}

/*
 map[
        _id:map[_data:826306F004000000032B022C0100296E5A1004EC0A378B4B3044C28DF4F18548BC3974463C5F6964003C6462746573746170702D6262346334636462342D6A687A6C7A000004]
        clusterTime:{1661399044 3}
        documentKey:map[_id:dbtestapp-bb4c4cdb4-jhzlz]
        ns:map[coll:ngapid db:sdcore]
        operationType:insert
        fullDocument:map[_id:dbtestapp-bb4c4cdb4-jhzlz expireAt:1661399064504 podId:dbtestapp-bb4c4cdb4-jhzlz time:1661399044 type:keepalive]
    ]

map[
        _id:map[_data:826306FE49000000012B022C0100296E5A10045287202787774B43958F3929CFD344D0463C5F6964003C6462746573746170702D3862396634383866372D6337347366000004]
        clusterTime:{1661402697 1}
        documentKey:map[_id:dbtestapp-8b9f488f7-c74sf]
        ns:map[coll:ngapid db:sdcore]
        operationType:update
        updateDescription:map[removedFields:[] updatedFields:map[expireAt:1661402717758 time:1661402697]]
   ]

map[
        _id:map[_data:82630701E5000000012B022C0100296E5A10045287202787774B43958F3929CFD344D0463C5F6964003C6462746573746170702D3862396634383866372D6E64327470000004]
        clusterTime:{1661403621 1}
        documentKey:map[_id:dbtestapp-8b9f488f7-nd2tp]
        ns:map[coll:ngapid db:sdcore]
        operationType:delete
   ]

map[
        _id:map[_data:826307FF400000000B2B022C0100296E5A1004020E4568089B4D8889A42D53E225B5AE463C5F6964003C6368756E6B69642D3131353638000004]
        clusterTime:{1661468480 11}
        documentKey:map[_id:chunkid-11568]
        fullDocument:map[_id:chunkid-11568 podId:dbtestapp-8644b5b7d6-qdk54 type:chunk]
        ns:map[coll:ngapid db:sdcore]
        operationType:insert]


map[
        _id:map[_data:8263085773000000022B022C0100296E5A1004E23062383C624633BDEE5B9B5FEAB2B8463C5F6964003C6368756E6B69642D38333332000004]
        clusterTime:{1661491059 2}
        documentKey:map[_id:chunkid-8332]
        ns:map[coll:ngapid db:sdcore]
        operationType:update
        updateDescription:map[removedFields:[] updatedFields:map[podId:dbtestapp-6dc68f9f68-7fwj8]]]

*/

// handle incoming db notification and update
func (d *Drsm) handleDbUpdates() {
	collection := d.mongo.GetCollection(d.sharedPoolName)

	// TODO : 2 go routines to monitor 2 pipelines
	pipeline := mongo.Pipeline{}

	for {
		// create stream to monitor actions on the collection
		updateStream, err := collection.Watch(context.TODO(), pipeline)
		if err != nil {
			time.Sleep(5000 * time.Millisecond)
			continue
		}
		routineCtx, _ := context.WithCancel(context.Background())
		// run routine to get messages from stream
		iterateChangeStream(d, routineCtx, updateStream)
	}
}

func iterateChangeStream(d *Drsm, routineCtx context.Context, stream *mongo.ChangeStream) {
	logger.DrsmLog.Debugf("iterate change stream for podData: %v", d)

	// step 1: Get Pod Keepalive triggers and create POD table
	// case 2: Update Global Chunk Table.
	// case 3: New POD addition
	// case 4: New chunk addition
	// case 5: POD down - keepalive doc deleted. Then inform Claim go routine.
	// case 6: Chunk owner change - claim

	defer stream.Close(routineCtx)
	for stream.Next(routineCtx) {
		var data bson.M
		if err := stream.Decode(&data); err != nil {
			panic(err)
		}
		var s streamDoc
		bsonBytes, _ := bson.Marshal(data)
		bson.Unmarshal(bsonBytes, &s)
		// logger.DrsmLog.Debugf("iterate stream : ", data)
		// logger.DrsmLog.Debugf("\ndecoded stream bson %+v \n", s)
		switch s.OpType {
		case "insert":
			full := &s.Full
			switch full.Type {
			case "keepalive":
				// logger.DrsmLog.Debugf("insert keepalive document")
				pod, found := d.podMap[full.PodId]
				if !found {
					d.addPod(full)
				} else {
					logger.DrsmLog.Debugln("keepalive insert document: found existing podId", pod)
				}
			case "chunk":
				// logger.DrsmLog.Debugln("insert chunk document")
				d.addChunk(full)
			}
		case "update":
			// chunk ownership changed..update chunk owner
			// logger.DrsmLog.Debugln("update operations")
			if isChunkDoc(s.DId.Id) {
				// update on chunkId..
				// looks like chunk owner getting change
				owner := s.Update.UpdFields.PodId
				c := getChunkIdFromDocId(s.DId.Id)
				d.globalChunkTblMutex.Lock()
				cp := d.globalChunkTbl[c]
				d.globalChunkTblMutex.Unlock()
				// TODO update IP address as well.
				cp.Owner.PodName = owner
				cp.Owner.PodIp = s.Update.UpdFields.PodIp
				cp.Owner.PodInstance = s.Update.UpdFields.PodInstance
				podD := d.podMap[owner]
				podD.podChunks[c] = cp // add chunk to pod
				logger.DrsmLog.Infof("stream(Update): pod to chunk map %v", podD.podChunks)
			}
		case "delete":
			logger.DrsmLog.Debugln("delete operations")
			if !isChunkDoc(s.DId.Id) {
				// not chunk type doc. So its POD doc.
				// delete only gets document id
				pod, found := d.podMap[s.DId.Id]
				if pod != nil {
					logger.DrsmLog.Infof("Stream(Delete): Pod %v and found %v. Chunks owned by crashed pod = %v", pod, found, pod.podChunks)
					d.podDown <- s.DId.Id
				}
			}
		}
	}
}

// periodic task
func (d *Drsm) punchLiveness() {
	// write to DB - signature every 5 second
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	logger.DrsmLog.Debugln("document expiry enabled")
	ret := d.mongo.RestfulAPICreateTTLIndex(d.sharedPoolName, 0, "expireAt")
	if ret {
		logger.DrsmLog.Debugln("ttl index created for Field: expireAt in Collection")
	} else {
		logger.DrsmLog.Debugln("ttl index exists for Field: expireAt in Collection")
	}

	for range ticker.C {
		// logger.DrsmLog.Debugln("update keepalive time")
		filter := bson.M{"_id": d.clientId.PodName}

		timein := time.Now().Local().Add(20 * time.Second)

		update := bson.D{
			{"_id", d.clientId.PodName},
			{"type", "keepalive"},
			{"podIp", d.clientId.PodIp},
			{"podId", d.clientId.PodName},
			{"podInstance", d.clientId.PodInstance},
			{"expireAt", timein},
		}

		_, err := d.mongo.PutOneCustomDataStructure(d.sharedPoolName, filter, update)
		if err != nil {
			logger.DrsmLog.Errorf("put data failed: %v", err)
			// TODO : should we panic ?
			continue
		}
	}
}

// periodic task
func (d *Drsm) checkAllChunks() {
	// go through all pods to see if any pod is showing same old counter
	// Mark it down locally
	// Claiming the chunks can be reactive
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		filter := bson.M{"type": "chunk"}
		result, err := d.mongo.RestfulAPIGetMany(d.sharedPoolName, filter)
		logger.DrsmLog.Debugf("chunk entry: %v", result)
		if err == nil && result != nil {
			for _, v := range result {
				var s FullStream
				bsonBytes, _ := bson.Marshal(v)
				bson.Unmarshal(bsonBytes, &s)
				logger.DrsmLog.Debugf("individual Chunk bson Element %v", s)
				d.addChunk(&s)
			}
		}
	}
}

func (d *Drsm) addChunk(full *FullStream) {
	pod, found := d.podMap[full.PodId]
	if !found {
		pod = d.addPod(full)
	}
	did := full.Id
	if did == "" {
		did = full.ChunkId
	}
	logger.DrsmLog.Debugf("received Chunk Doc: %v", full)
	cid := getChunkIdFromDocId(did)
	o := PodId{PodName: full.PodId, PodInstance: full.PodInstance, PodIp: full.PodIp}
	c := &chunk{Id: cid, Owner: o}
	c.resourceValidCb = d.resourceValidCb

	pod.podChunks[cid] = c

	d.globalChunkTblMutex.Lock()
	d.globalChunkTbl[cid] = c
	d.globalChunkTblMutex.Unlock()

	logger.DrsmLog.Debugf("chunk id %v, podChunks %v", cid, pod.podChunks)
}

func (d *Drsm) addPod(full *FullStream) *podData {
	podI := PodId{PodName: full.PodId, PodInstance: full.PodInstance, PodIp: full.PodIp}
	pod := &podData{PodId: podI}
	pod.podChunks = make(map[int32]*chunk)
	d.podMap[full.PodId] = pod
	logger.DrsmLog.Infof("keepalive insert d.podMaps %v", d.podMap)
	return pod
}
