// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"sync"

	"github.com/omec-project/nas/nasMessage"
	"github.com/omec-project/openapi/models"
)

type SmContext struct {
	mu sync.RWMutex // protect the following fields

	// pdu session information
	pduSessionID int32
	smContextRef string
	snssai       models.Snssai
	dnn          string
	accessType   models.AccessType
	nsInstance   string
	userLocation models.UserLocation
	plmnID       models.PlmnId

	// SMF information
	smfID  string
	smfUri string
	hSmfID string
	vSmfID string

	//status of pdusession
	pduSessionInactive bool

	// for duplicate pdu session id handling
	ulNASTransport *nasMessage.ULNASTransport
	duplicated     bool
}

func NewSmContext(pduSessionID int32) *SmContext {
	c := &SmContext{pduSessionID: pduSessionID}
	return c
}

func (c *SmContext) IsPduSessionActive() bool {
	return !c.pduSessionInactive
}

func (c *SmContext) SetPduSessionInActive(s bool) {
	c.pduSessionInactive = s
}

func (c *SmContext) PduSessionID() int32 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.pduSessionID
}

func (c *SmContext) SetPduSessionID(id int32) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pduSessionID = id
}

func (c *SmContext) SmContextRef() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.smContextRef
}

func (c *SmContext) SetSmContextRef(ref string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.smContextRef = ref
}

func (c *SmContext) AccessType() models.AccessType {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.accessType
}

func (c *SmContext) SetAccessType(accessType models.AccessType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.accessType = accessType
}

func (c *SmContext) Snssai() models.Snssai {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.snssai
}

func (c *SmContext) SetSnssai(snssai models.Snssai) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.snssai = snssai
}

func (c *SmContext) Dnn() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.dnn
}

func (c *SmContext) SetDnn(dnn string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dnn = dnn
}

func (c *SmContext) NsInstance() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nsInstance
}

func (c *SmContext) SetNsInstance(nsInstanceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nsInstance = nsInstanceID
}

func (c *SmContext) UserLocation() models.UserLocation {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.userLocation
}

func (c *SmContext) SetUserLocation(userLocation models.UserLocation) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.userLocation = userLocation
}

func (c *SmContext) PlmnID() models.PlmnId {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.plmnID
}

func (c *SmContext) SetPlmnID(plmnID models.PlmnId) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.plmnID = plmnID
}

func (c *SmContext) SmfID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.smfID
}

func (c *SmContext) SetSmfID(smfID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.smfID = smfID
}

func (c *SmContext) SmfUri() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.smfUri
}

func (c *SmContext) SetSmfUri(smfUri string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.smfUri = smfUri
}

func (c *SmContext) HSmfID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hSmfID
}

func (c *SmContext) SetHSmfID(hsmfID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hSmfID = hsmfID
}

func (c *SmContext) VSmfID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.vSmfID
}

func (c *SmContext) SetVSmfID(vsmfID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.vSmfID = vsmfID
}

func (c *SmContext) PduSessionIDDuplicated() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.duplicated
}

func (c *SmContext) SetDuplicatedPduSessionID(duplicated bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.duplicated = duplicated
}

func (c *SmContext) ULNASTransport() *nasMessage.ULNASTransport {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ulNASTransport
}

func (c *SmContext) StoreULNASTransport(msg *nasMessage.ULNASTransport) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ulNASTransport = msg
}

func (c *SmContext) DeleteULNASTransport() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ulNASTransport = nil
}
