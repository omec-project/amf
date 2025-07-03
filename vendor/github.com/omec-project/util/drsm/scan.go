// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
package drsm

import (
	"time"

	"github.com/omec-project/util/logger"
)

func (c *chunk) scanChunk(d *Drsm) {
	if d.mode == ResourceDemux {
		logger.DrsmLog.Infoln("do not perform scan task when demux mode is ON")
		return
	}

	if c.Owner.PodName != d.clientId.PodName {
		logger.DrsmLog.Infoln("do not perform scan task if Chunk is not owned by us")
		return
	}
	c.State = Scanning
	d.scanChunks[c.Id] = c
	var i int32
	for i = 0; i < 1000; i++ {
		c.ScanIds = append(c.ScanIds, i)
	}

	ticker := time.NewTicker(5000 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			logger.DrsmLog.Debugf("let's scan one by one id for %v, chunk details %v", c.Id, c)
			// TODO : find candidate and then scan that Id.
			// once all Ids are scanned then we can start using this block
			if c.resourceValidCb != nil {
				if len(c.ScanIds) != 0 {
					id := c.ScanIds[len(c.ScanIds)-1]
					c.ScanIds = c.ScanIds[:len(c.ScanIds)-1]
					rid := c.Id<<10 | id
					res := c.resourceValidCb(rid)
					if res {
						c.FreeIds = append(c.FreeIds, id)
					} else {
						c.AllocIds[id] = true // Id is in use
					}
				} else {
					// mark as owned. and remove from scan list and add to local table
					c.State = Owned
					d.localChunkTbl[c.Id] = c
					delete(d.scanChunks, c.Id)
					logger.DrsmLog.Debugf("scan complete for Chunk %v", c.Id)
					return
				}
			}
			// no one is writing on stopScan for now. We will use it eventually
		case <-c.stopScan:
			logger.DrsmLog.Debugf("received Stop Scan. Closing scan for %v", c.Id)
			return
		}
	}
}
