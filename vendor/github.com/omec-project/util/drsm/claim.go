// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
package drsm

import (
	"fmt"

	"github.com/omec-project/util/logger"
	"go.mongodb.org/mongo-driver/bson"
)

func (d *Drsm) podDownDetected() {
	logger.DrsmLog.Infoln("started Pod Down goroutine")
	for p := range d.podDown {
		logger.DrsmLog.Infof("pod Down detected %v", p)
		// Given Pod find out current Chunks owned by this POD
		pd := d.podMap[p]
		for k := range pd.podChunks {
			d.globalChunkTblMutex.Lock()
			c, found := d.globalChunkTbl[k]
			d.globalChunkTblMutex.Unlock()
			logger.DrsmLog.Debugf("found: %v chunk: %v", found, c)
			if found {
				go c.claimChunk(d, pd.PodId.PodName)
			}
		}
	}
}

func (c *chunk) claimChunk(d *Drsm, curOwner string) {
	// Need optimization
	if d.mode != ResourceClient {
		logger.DrsmLog.Infoln("claimChunk ignored demux mode")
		return
	}
	// try to claim. If success then notification will update owner.
	logger.DrsmLog.Debugln("claimChunk started")
	docId := fmt.Sprintf("chunkid-%d", c.Id)
	update := bson.M{"_id": docId, "type": "chunk", "podId": d.clientId.PodName, "podInstance": d.clientId.PodInstance, "podIp": d.clientId.PodIp}
	filter := bson.M{"_id": docId, "podId": curOwner}
	updated := d.mongo.RestfulAPIPutOnly(d.sharedPoolName, filter, update)
	if updated == nil {
		// TODO : don't add to local pool yet. We can add it only if scan is done.
		logger.DrsmLog.Infof("claimChunk %v success", c.Id)
		c.Owner.PodName = d.clientId.PodName
		c.Owner.PodIp = d.clientId.PodIp
		go c.scanChunk(d)
	} else {
		// no problem, some other POD successfully claimed this chunk
		logger.DrsmLog.Infof("claimChunk %v failure", c.Id)
	}
}
