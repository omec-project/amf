// SPDX-FileCopyrightText: 2022 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0

package drsm

import (
	"fmt"
	"sync"

	"github.com/omec-project/util/logger"
)

type DbInfo struct {
	Url  string
	Name string
}

type PodId struct {
	PodName     string `bson:"podName,omitempty" json:"podName,omitempty"`
	PodInstance string `bson:"podInstance,omitempty" json:"podInstance,omitempty"`
	PodIp       string `bson:"podIp,omitempty" json:"podIp,omitempty"`
}

type DrsmMode int

var mutex sync.Mutex

const (
	ResourceClient DrsmMode = iota + 0
	ResourceDemux
)

type Options struct {
	ResIdSize       int32 // size in bits e.g. 32 bit, 24 bit.
	Mode            DrsmMode
	ResourceValidCb func(int32) bool // return if ID is in use or not used
	IpPool          map[string]string
}

type DrsmInterface interface {
	AllocateInt32ID() (int32, error)
	ReleaseInt32ID(id int32) error
	FindOwnerInt32ID(id int32) (*PodId, error)
	AcquireIp(pool string) (string, error)
	ReleaseIp(pool, ip string) error
	CreateIpPool(poolName string, ipPool string) error
	DeleteIpPool(poolName string) error
	DeletePod(string)
}

func InitDRSM(sharedPoolName string, myid PodId, db DbInfo, opt *Options) (DrsmInterface, error) {
	logger.DrsmLog.Infoln("client id:", myid)

	d := &Drsm{
		sharedPoolName: sharedPoolName,
		clientId:       myid,
		db:             db,
		mode:           ResourceClient,
	}

	d.ConstuctDrsm(opt)

	return d, nil
}

func (d *Drsm) AllocateInt32ID() (int32, error) {
	mutex.Lock()
	defer mutex.Unlock()
	if d.mode == ResourceDemux {
		logger.DrsmLog.Errorln("demux mode can not allocate Resource index")
		err := fmt.Errorf("demux mode does not allow Resource Id allocation")
		return 0, err
	}
	for _, c := range d.localChunkTbl {
		if len(c.FreeIds) > 0 {
			return c.AllocateIntID()
		}
	}
	// None of the Chunk has freeIds. Allocate new Chunk
	c, err := d.GetNewChunk()
	if err != nil {
		logger.DrsmLog.Errorln("failed to allocate new Chunk")
		err := fmt.Errorf("failed to allocate new Chunk")
		return 0, err
	}
	return c.AllocateIntID()
}

func (d *Drsm) ReleaseInt32ID(id int32) error {
	mutex.Lock()
	defer mutex.Unlock()
	if d.mode == ResourceDemux {
		logger.DrsmLog.Debugln("demux mode can not release Resource index")
		err := fmt.Errorf("demux mode does not allow Resource Id allocation")
		return err
	}

	chunkId := id >> 10
	chunk, found := d.localChunkTbl[chunkId]
	if found {
		chunk.ReleaseIntID(id)
		logger.DrsmLog.Debugln("id released:", id)
		return nil
	} else {
		chunk, found := d.scanChunks[chunkId]
		if found {
			chunk.ReleaseIntID(id)
			return nil
		}
	}
	logger.DrsmLog.Errorf("failed to release id - %v", id)
	return fmt.Errorf("unknown Id")
}

func (d *Drsm) FindOwnerInt32ID(id int32) (*PodId, error) {
	d.globalChunkTblMutex.Lock()
	defer d.globalChunkTblMutex.Unlock()
	chunkId := id >> 10
	chunk, found := d.globalChunkTbl[chunkId]
	if found {
		podId := chunk.GetOwner()
		return podId, nil
	}
	logger.DrsmLog.Errorf("failed to find POD owner for Id - %v ", id)
	return nil, fmt.Errorf("unknown Id")
}

func (d *Drsm) AcquireIp(pool string) (string, error) {
	if d.mode == ResourceDemux {
		logger.DrsmLog.Errorln("demux mode can not allocate Ip")
		return "", fmt.Errorf("demux mode does not allow Resource allocation")
	}
	return d.acquireIp(pool)
}

func (d *Drsm) ReleaseIp(pool, ip string) error {
	if d.mode == ResourceDemux {
		logger.DrsmLog.Errorln("demux mode can not Release Resource")
		return fmt.Errorf("demux mode does not allow Resource Release")
	}
	return d.releaseIp(pool, ip)
}

func (d *Drsm) CreateIpPool(poolName string, ipPool string) error {
	err := d.initIpPool(poolName, ipPool)
	return err
}

func (d *Drsm) DeleteIpPool(poolName string) error {
	err := d.deleteIpPool(poolName)
	return err
}
