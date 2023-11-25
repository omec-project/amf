// SPDX-FileCopyrightText: 2022-present Intel Corporation
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
	Mu sync.RWMutex // protect the following fields

	// pdu session information
	PduSessionIDVal int32
	SmContextRefVal string
	SnssaiVal       models.Snssai
	DnnVal          string
	AccessTypeVal   models.AccessType
	NsInstanceVal   string
	UserLocationVal models.UserLocation
	PlmnIDVal       models.PlmnId

	// SMF information
	SmfIDVal  string
	SmfUriVal string
	HSmfIDVal string
	VSmfIDVal string

	// status of pdusession
	PduSessionInactiveVal bool

	// for duplicate pdu session id handling
	UlNASTransportVal *nasMessage.ULNASTransport
	DuplicatedVal     bool

	SmfProfiles []models.NfProfile
}

func NewSmContext(pduSessionID int32) *SmContext {
	c := &SmContext{PduSessionIDVal: pduSessionID}
	return c
}

func (c *SmContext) IsPduSessionActive() bool {
	return !c.PduSessionInactiveVal
}

func (c *SmContext) SetPduSessionInActive(s bool) {
	c.PduSessionInactiveVal = s
}

func (c *SmContext) PduSessionID() int32 {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.PduSessionIDVal
}

func (c *SmContext) SetPduSessionID(id int32) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.PduSessionIDVal = id
}

func (c *SmContext) SmContextRef() string {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.SmContextRefVal
}

func (c *SmContext) SetSmContextRef(ref string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.SmContextRefVal = ref
}

func (c *SmContext) AccessType() models.AccessType {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.AccessTypeVal
}

func (c *SmContext) SetAccessType(accessType models.AccessType) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.AccessTypeVal = accessType
}

func (c *SmContext) Snssai() models.Snssai {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.SnssaiVal
}

func (c *SmContext) SetSnssai(snssai models.Snssai) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.SnssaiVal = snssai
}

func (c *SmContext) Dnn() string {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.DnnVal
}

func (c *SmContext) SetDnn(dnn string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.DnnVal = dnn
}

func (c *SmContext) NsInstance() string {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.NsInstanceVal
}

func (c *SmContext) SetNsInstance(nsInstanceID string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.NsInstanceVal = nsInstanceID
}

func (c *SmContext) UserLocation() models.UserLocation {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.UserLocationVal
}

func (c *SmContext) SetUserLocation(userLocation models.UserLocation) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.UserLocationVal = userLocation
}

func (c *SmContext) PlmnID() models.PlmnId {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.PlmnIDVal
}

func (c *SmContext) SetPlmnID(plmnID models.PlmnId) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.PlmnIDVal = plmnID
}

func (c *SmContext) SmfID() string {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.SmfIDVal
}

func (c *SmContext) SetSmfID(smfID string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.SmfIDVal = smfID
}

func (c *SmContext) SmfUri() string {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.SmfUriVal
}

func (c *SmContext) SetSmfUri(smfUri string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.SmfUriVal = smfUri
}

func (c *SmContext) HSmfID() string {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.HSmfIDVal
}

func (c *SmContext) SetHSmfID(hsmfID string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.HSmfIDVal = hsmfID
}

func (c *SmContext) VSmfID() string {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.VSmfIDVal
}

func (c *SmContext) SetVSmfID(vsmfID string) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.VSmfIDVal = vsmfID
}

func (c *SmContext) PduSessionIDDuplicated() bool {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.DuplicatedVal
}

func (c *SmContext) SetDuplicatedPduSessionID(duplicated bool) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.DuplicatedVal = duplicated
}

func (c *SmContext) ULNASTransport() *nasMessage.ULNASTransport {
	c.Mu.RLock()
	defer c.Mu.RUnlock()
	return c.UlNASTransportVal
}

func (c *SmContext) StoreULNASTransport(msg *nasMessage.ULNASTransport) {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.UlNASTransportVal = msg
}

func (c *SmContext) DeleteULNASTransport() {
	c.Mu.Lock()
	defer c.Mu.Unlock()
	c.UlNASTransportVal = nil
}
