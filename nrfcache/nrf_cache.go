// SPDX-FileCopyrightText: 2022 Infosys Limited
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nrf_cache

import (
	"container/heap"
	"encoding/json"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/models"
	"sync"
	"time"
)

const defaultCacheTTl = time.Hour
const defaultNfProfileTTl = time.Minute

type NfProfileItem struct {
	nfProfile  *models.NfProfile
	ttl        time.Duration
	expiryTime time.Time
	index      int // index of the entry in the priority queue
}

func (item *NfProfileItem) isExpired() bool {
	return item.expiryTime.Before(time.Now())
}

func (item *NfProfileItem) updateExpiryTime() {
	item.expiryTime = time.Now().Add(time.Second * item.ttl)
}

func newNfProfileItem(profile *models.NfProfile, ttl time.Duration) *NfProfileItem {
	item := &NfProfileItem{
		nfProfile: profile,
		ttl:       ttl,
	}
	// since nobody is aware yet of this item, it's safe to touch without lock here
	item.updateExpiryTime()
	return item
}

// NfProfilePriorityQ : Priority Queue to store the profile by expiry time
type NfProfilePriorityQ []*NfProfileItem

func (npq NfProfilePriorityQ) Len() int {
	return len(npq)
}

func (npq NfProfilePriorityQ) Less(i, j int) bool {
	return npq[i].expiryTime.Before(npq[j].expiryTime)
}

func (npq NfProfilePriorityQ) Swap(i, j int) {
	npq[i], npq[j] = npq[j], npq[i]
	npq[i].index = i
	npq[j].index = j
}

func (npq NfProfilePriorityQ) root() *NfProfileItem {
	return npq[0]
}

func (npq NfProfilePriorityQ) at(index int) *NfProfileItem {
	return npq[index]
}

func (npq *NfProfilePriorityQ) push(item interface{}) {
	heap.Push(npq, item)
}

func (npq *NfProfilePriorityQ) pop() interface{} {
	if npq.Len() == 0 {
		return nil
	}
	return heap.Pop(npq).(*NfProfileItem)
}

// update modifies the priority and value of an Item in the queue.
func (npq *NfProfilePriorityQ) update(item *NfProfileItem, value *models.NfProfile, ttl time.Duration) {
	item.nfProfile = value
	item.ttl = ttl
	item.updateExpiryTime()
	heap.Fix(npq, item.index)
}

func (npq *NfProfilePriorityQ) remove(item *NfProfileItem) {
	heap.Remove(npq, item.index)
}

func (npq *NfProfilePriorityQ) Push(item interface{}) {
	n := len(*npq)
	entry := item.(*NfProfileItem)
	entry.index = n
	*npq = append(*npq, entry)
}

func (npq *NfProfilePriorityQ) Pop() interface{} {
	old := *npq
	n := len(old)
	item := old[n-1]
	old[n-1] = nil
	item.index = -1
	*npq = old[0 : n-1]
	return item
}

func newNfProfilePriorityQ() *NfProfilePriorityQ {
	q := &NfProfilePriorityQ{}
	heap.Init(q)
	return q
}

type NrfRequest struct {
	targetNfType models.NfType
	searchParams *Nnrf_NFDiscovery.SearchNFInstancesParamOpts
}

// NrfCache : cache of nf profiles
type NrfCache struct {
	cache map[string]*NfProfileItem // map[nf-instance-id] =*NfProfile

	priorityQ *NfProfilePriorityQ // sorted by expiry time

	evictionInterval time.Duration // timer interval in which the cache is checked for eviction of expired entries

	evictionTicker *time.Ticker

	nrfDiscoveryQueryCb NrfDiscoveryQueryCb // nrf query callback

	done chan struct{}

	mutex sync.RWMutex
}

func (c *NrfCache) handleLookup(nrfUri string, targetNfType, requestNfType models.NfType, param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) (models.SearchResult, error) {
	var searchResult models.SearchResult
	var err error

	c.mutex.RLock()
	searchResult.NfInstances = c.get(param)
	c.mutex.RUnlock()

	if len(searchResult.NfInstances) == 0 {
		logger.UtilLog.Tracef("Cache miss for nftype %s", targetNfType)

		c.mutex.Lock()
		searchResult.NfInstances = c.get(param)
		if len(searchResult.NfInstances) == 0 {
			searchResult, err = c.nrfDiscoveryQueryCb(nrfUri, targetNfType, requestNfType, param)
			if err != nil {
				return searchResult, err
			}

			for i := 0; i < len(searchResult.NfInstances); i++ {
				c.set(&searchResult.NfInstances[i], time.Duration(searchResult.ValidityPeriod))
			}
		}
		c.mutex.Unlock()
	}
	return searchResult, err
}

func (c *NrfCache) set(nfProfile *models.NfProfile, ttl time.Duration) {
	if ttl == 0 {
		ttl = defaultNfProfileTTl
	}

	item, exists := c.cache[nfProfile.NfInstanceId]
	if exists {
		// if item.isExpired()
		c.priorityQ.update(item, nfProfile, ttl)
	} else {
		newItem := newNfProfileItem(nfProfile, ttl)
		c.cache[nfProfile.NfInstanceId] = newItem
		c.priorityQ.push(newItem)
	}
}

func (c *NrfCache) get(opts *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) []models.NfProfile {
	var nfProfiles []models.NfProfile

	for _, element := range c.cache {
		if !element.isExpired() {
			if opts != nil {
				cb, ok := matchFilters[element.nfProfile.NfType]
				if ok && cb(element.nfProfile, opts) {
					nfProfiles = append(nfProfiles, *(element.nfProfile))
				}
			} else {
				nfProfiles = append(nfProfiles, *(element.nfProfile))
			}
		}
	}
	return nfProfiles
}

func (c *NrfCache) remove(item *NfProfileItem) {
	c.priorityQ.remove(item)
	delete(c.cache, item.nfProfile.NfInstanceId)
}

func (c *NrfCache) cleanupExpiredItems() {
	logger.UtilLog.Infoln("nrf cache: cleanup expired items")

	for item := c.priorityQ.at(0); item.isExpired(); {

		logger.UtilLog.Tracef("evicted nf instance %s", item.nfProfile.NfInstanceId)

		c.remove(item)
		if c.priorityQ.Len() == 0 {
			break
		} else {
			item = c.priorityQ.at(0)
		}
	}
}

func (c *NrfCache) purge() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	close(c.done)
	c.priorityQ = newNfProfilePriorityQ()
	c.cache = make(map[string]*NfProfileItem)
	c.evictionTicker.Stop()
}

func (c *NrfCache) startExpiryProcessing() {
	for {
		select {
		case <-c.evictionTicker.C:
			c.mutex.Lock()
			if c.priorityQ.Len() == 0 {
				c.mutex.Unlock()
				continue
			}

			c.cleanupExpiredItems()
			c.mutex.Unlock()

		case <-c.done:
			return
		}
	}
}

func NewNrfCache(duration time.Duration, dbqueryCb NrfDiscoveryQueryCb) *NrfCache {
	cache := &NrfCache{
		cache:               make(map[string]*NfProfileItem),
		priorityQ:           newNfProfilePriorityQ(),
		evictionInterval:    defaultCacheTTl,
		nrfDiscoveryQueryCb: dbqueryCb,
		done:                make(chan struct{}),
	}

	cache.evictionTicker = time.NewTicker(duration)

	go cache.startExpiryProcessing()

	return cache
}

func copyNrfProfile(src *models.NfProfile) (*models.NfProfile, error) {
	nrfProfileJSON, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}
	nrfProfile := models.NfProfile{}
	if err = json.Unmarshal(nrfProfileJSON, &nrfProfile); err != nil {
		return nil, err
	}

	return &nrfProfile, nil
}

type NrfMasterCache struct {
	nfTypeToCacheMap    map[models.NfType]*NrfCache
	evictionInterval    time.Duration
	nrfDiscoveryQueryCb NrfDiscoveryQueryCb

	mutex sync.Mutex
}

func (c *NrfMasterCache) GetNrfCacheInstance(targetNfType models.NfType) *NrfCache {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cache, exists := c.nfTypeToCacheMap[targetNfType]
	if exists == false {
		logger.UtilLog.Infof("Creating cache for nftype %v", targetNfType)

		cache = NewNrfCache(c.evictionInterval, c.nrfDiscoveryQueryCb)
		c.nfTypeToCacheMap[targetNfType] = cache
	}
	return cache
}

func (c *NrfMasterCache) clearNrfCache(nfType models.NfType) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	cache, exists := c.nfTypeToCacheMap[nfType]
	if exists == true {
		cache.purge()
		delete(c.nfTypeToCacheMap, nfType)
	}
}

func (c *NrfMasterCache) clearNrfMasterCache() {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for k, cache := range c.nfTypeToCacheMap {
		cache.purge()
		delete(c.nfTypeToCacheMap, k)
	}
}

var masterCache *NrfMasterCache

type NrfDiscoveryQueryCb func(nrfUri string, targetNfType, requestNfType models.NfType, param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) (models.SearchResult, error)

func InitNrfCaching(interval time.Duration, cb NrfDiscoveryQueryCb) {
	m := &NrfMasterCache{
		nfTypeToCacheMap:    make(map[models.NfType]*NrfCache),
		evictionInterval:    interval,
		nrfDiscoveryQueryCb: cb,
	}
	masterCache = m
}

func DisableNrfCaching() {
	masterCache.clearNrfMasterCache()
	masterCache = nil
}

func SearchNFInstances(nrfUri string, targetNfType, requestNfType models.NfType, param *Nnrf_NFDiscovery.SearchNFInstancesParamOpts) (models.SearchResult, error) {

	logger.UtilLog.Traceln("SearchNFInstances nrf cache")

	var searchResult models.SearchResult
	var err error

	c := masterCache.GetNrfCacheInstance(targetNfType)
	if c != nil {
		searchResult, err = c.handleLookup(nrfUri, targetNfType, requestNfType, param)
	} else {
		logger.UtilLog.Errorln("Failed to find cache for nftype")
	}

	for _, np := range searchResult.NfInstances {
		logger.UtilLog.Tracef("%v", np)
	}

	return searchResult, err

}
