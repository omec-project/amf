// SPDX-FileCopyrightText: 2022-present Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/ngapconv"
	"github.com/omec-project/ngap/v2/ngapConvert"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
	"go.uber.org/zap"
)

type RelAction int

const (
	RanUeNgapIdUnspecified int64 = 0xffffffff
)

const (
	UeContextN2NormalRelease RelAction = iota
	UeContextReleaseHandover
	UeContextReleaseUeContext
	UeContextReleaseDueToNwInitiatedDeregistraion
	// UeContextReleaseDueToUeInitiatedDeregistration is distinct from
	// UeContextReleaseUeContext because that action must keep a stored context when a valid
	// security context exists, and both of its senders rely on that guard.
	// HandleSecurityModeReject clears SecurityContextAvailable immediately before releasing,
	// so it takes the delete branch. HandleUEContextReleaseRequest sends this action only for
	// an access that is not GMM-Registered -- a UE still mid-registration, which has held a
	// security context since the Security Mode Command was built, or one still registered over
	// the other access; a Registered access gets UeContextN2NormalRelease instead. Neither of
	// those UEs has left the network, so their stored context has to survive.
	// A UE with no access left in use has left the network, and keeping its context leaves a
	// stored document that its next registration will meet; while the other access is still
	// in use, only the named access is released.
	UeContextReleaseDueToUeInitiatedDeregistration
)

type RanUe struct {
	/* UE identity*/
	RanUeNgapId int64 `json:"ranUeNgapId,omitempty"`
	AmfUeNgapId int64 `json:"amfUeNgapId,omitempty"`

	/* HandOver Info*/
	HandOverType        ngapType.HandoverType
	SuccessPduSessionId []int32 `json:"successPduSessionId,omitempty"`
	SourceUe            *RanUe  `json:"-"`
	TargetUe            *RanUe  `json:"-"`

	/* UserLocation*/
	Tai      models.Tai
	Location models.UserLocation
	/* context about udm */
	SupportVoPSn3gpp  bool       `json:"-"`
	SupportVoPS       bool       `json:"-"`
	SupportedFeatures string     `json:"-"`
	LastActTime       *time.Time `json:"-"`

	/* Related Context*/
	// AmfUe is cleared from other goroutines (Remove, DetachAmfUe) while the NGAP
	// read loop is still dispatching messages for this RanUe, so it is guarded by
	// amfUeMu and must be reached through GetAmfUe/SetAmfUe rather than directly.
	AmfUe   *AmfUe `json:"-"`
	amfUeMu sync.RWMutex
	Ran     *AmfRan
	// listing orders a RAN listing this RanUe against a replacement taking it out of
	// RanUePool and against its removal; see listOn. A listing by listOn changes Ran and
	// RanUeNgapId under it, and Remove reads them under it; handlers that set them directly
	// do not take it.
	listing sync.Mutex
	// restoredUnlisted: a restore published this RanUe in RanUePool, and no RAN has listed it
	// since. unpooled: a replacement took it out of RanUePool while it was unlisted. removed:
	// Remove has started on it.
	restoredUnlisted bool
	unpooled         bool
	removed          bool

	/* Routing ID */
	RoutingID string
	/* Trace Recording Session Reference */
	Trsr string
	/* Ue Context Release Action */
	ReleaseAction RelAction
	/* context used for AMF Re-allocation procedure */
	OldAmfName            string
	InitialUEMessage      []byte
	RRCEstablishmentCause string // Received from initial ue message; pattern: ^[0-9a-fA-F]+$
	UeContextRequest      bool

	/* send initial context setup request or not*/
	SentInitialContextSetupRequest bool

	/*Received Initial context setup response or not */
	RecvdInitialContextSetupResponse bool

	/* logger */
	Log *zap.SugaredLogger `json:"-"`

	/* Sctplb Redirect Msg */
	SctplbMsg []byte

	// replacements counts the times an AttachRanUe has replaced this RanUe, so that only the
	// latest replacement's grace period ends in a detach. Each UE counts under its own lock,
	// and a RanUe can move between UEs, so the count is atomic.
	replacements atomic.Uint64
}

func (ranUe *RanUe) Remove() error {
	logger.ContextLog.Infoln("RanUe has been deleted")
	if ranUe == nil {
		return fmt.Errorf("RanUe not found in RemoveRanUe")
	}
	ran, ranUeNgapID := ranUe.markRemoved()
	if ran == nil {
		return fmt.Errorf("RanUe not found in Ran")
	}
	if amfUe := ranUe.GetAmfUe(); amfUe != nil {
		amfUe.Mutex.Lock()
		if amfUe.RanUe[ran.AnType] == ranUe {
			delete(amfUe.RanUe, ran.AnType)
		}
		ranUe.DetachAmfUeIf(amfUe)
		amfUe.Mutex.Unlock()
	}

	ran.ranStateMu.Lock()
	delete(ran.RanUeList, ranUeNgapID)
	ran.ranStateMu.Unlock()
	self := AMF_Self()
	if self.EnableDbStore {
		if err := self.Drsm.ReleaseInt32ID(int32(ranUe.AmfUeNgapId)); err != nil {
			logger.ContextLog.Errorf("error releasing UE: %v", err)
		}
	} else {
		amfUeNGAPIDGenerator.FreeID(ranUe.AmfUeNgapId)
	}
	self.RanUePool.Delete(ranUe.AmfUeNgapId)
	return nil
}

// GetAmfUe returns the AmfUe this RanUe belongs to, or nil once the association has
// been dropped. Callers must keep the returned pointer instead of re-reading the
// field. Every re-read is a fresh chance to observe the nil that Remove writes, and
// the NGAP read loop that reads it runs with no recover(), so one badly-timed re-read
// ends the whole AMF process rather than one procedure.
func (ranUe *RanUe) GetAmfUe() *AmfUe {
	if ranUe == nil {
		return nil
	}

	ranUe.amfUeMu.RLock()
	defer ranUe.amfUeMu.RUnlock()

	return ranUe.AmfUe
}

// SetAmfUe associates this RanUe with amfUe.
func (ranUe *RanUe) SetAmfUe(amfUe *AmfUe) {
	if ranUe == nil {
		return
	}

	ranUe.amfUeMu.Lock()
	defer ranUe.amfUeMu.Unlock()

	ranUe.AmfUe = amfUe
}

func (ranUe *RanUe) DetachAmfUe() {
	if ranUe == nil {
		return
	}

	ranUe.amfUeMu.Lock()
	defer ranUe.amfUeMu.Unlock()

	ranUe.AmfUe = nil
}

// DetachAmfUeIf drops the association only while it still points at amfUe, so a
// release that overlaps a re-attach to a different context cannot undo the newer one.
// It reports whether it dropped it.
func (ranUe *RanUe) DetachAmfUeIf(amfUe *AmfUe) bool {
	if ranUe == nil {
		return false
	}

	ranUe.amfUeMu.Lock()
	defer ranUe.amfUeMu.Unlock()

	if ranUe.AmfUe != amfUe {
		return false
	}
	ranUe.AmfUe = nil

	return true
}

// markRestored records that a restore has published this RanUe in RanUePool, on no RAN's list.
func (ranUe *RanUe) markRestored() {
	ranUe.listing.Lock()
	defer ranUe.listing.Unlock()

	ranUe.restoredUnlisted = true
}

// unpoolIfRestoredUnlisted takes this RanUe out of RanUePool, by identity, if a restore
// published it and no RAN has listed it since.
func (ranUe *RanUe) unpoolIfRestoredUnlisted() {
	ranUe.listing.Lock()
	defer ranUe.listing.Unlock()

	if ranUe.restoredUnlisted && AMF_Self().RanUePool.CompareAndDelete(ranUe.AmfUeNgapId, ranUe) {
		ranUe.unpooled = true
	}
}

// markRemoved records that Remove has started on this RanUe, so that no RAN lists it and no
// listing publishes it again after this, and returns the RAN and RAN-UE-NGAP-ID it is listed
// under, read under the lock a listing changes them under. A RanUe with no RAN is not marked;
// Remove leaves it as it is.
func (ranUe *RanUe) markRemoved() (*AmfRan, int64) {
	ranUe.listing.Lock()
	defer ranUe.listing.Unlock()

	if ranUe.Ran != nil {
		ranUe.removed = true
	}

	return ranUe.Ran, ranUe.RanUeNgapId
}

// listedAt returns the RAN and RAN-UE-NGAP-ID this RanUe is listed under.
func (ranUe *RanUe) listedAt() (*AmfRan, int64) {
	ranUe.listing.Lock()
	defer ranUe.listing.Unlock()

	return ranUe.Ran, ranUe.RanUeNgapId
}

// listOn lists this RanUe on ran under ranUeNgapID and makes them its RAN and RAN-UE-NGAP-ID,
// unless Remove has started on it or its AMF-UE-NGAP-ID names another RanUe in RanUePool, and
// reports whether it did. The caller holds ran's ranStateMu. It clears the restore's mark and,
// if a replacement took the RanUe out of RanUePool while it was unlisted, publishes it again,
// since its release through this RAN has to find it. A RanUe whose id has gone to another one
// is listed nowhere: a release naming that id would reach the other RanUe. The removal from
// RanUePool and Remove take the same lock, so whichever comes second sees what the first did.
func (ranUe *RanUe) listOn(ran *AmfRan, ranUeNgapID int64) bool {
	ranUe.listing.Lock()
	defer ranUe.listing.Unlock()

	if ranUe.removed {
		return false
	}
	pool := &AMF_Self().RanUePool
	if ranUe.unpooled {
		if owner, loaded := pool.LoadOrStore(ranUe.AmfUeNgapId, ranUe); loaded && owner != ranUe {
			return false
		}
		ranUe.unpooled = false
	} else if owner, ok := pool.Load(ranUe.AmfUeNgapId); ok && owner != ranUe {
		return false
	}
	ranUe.restoredUnlisted = false
	ran.RanUeList[ranUeNgapID] = ranUe
	ranUe.Ran = ran
	ranUe.RanUeNgapId = ranUeNgapID

	return true
}

func (ranUe *RanUe) SwitchToRan(newRan *AmfRan, ranUeNgapId int64) error {
	if ranUe == nil {
		return fmt.Errorf("ranUe is nil")
	}

	if newRan == nil {
		return fmt.Errorf("newRan is nil")
	}

	oldRan, oldRanUeNgapID := ranUe.listedAt()

	// remove ranUe from oldRan
	oldRan.ranStateMu.Lock()
	delete(oldRan.RanUeList, oldRanUeNgapID)
	oldRan.ranStateMu.Unlock()

	// add ranUe to newRan, and switch to it
	newRan.ranStateMu.Lock()
	listed := ranUe.listOn(newRan, ranUeNgapId)
	newRan.ranStateMu.Unlock()
	if !listed {
		return fmt.Errorf("RanUe[AmfUeNgapID: %d] cannot switch to Ran[Name: %s]: it was removed, "+
			"or its AmfUeNgapID names another RanUe", ranUe.AmfUeNgapId, newRan.Name)
	}

	logger.ContextLog.Infof("RanUe[RanUeNgapID: %d] Switch to new Ran[Name: %s]", ranUeNgapId, newRan.Name)
	return nil
}

// sameTai reports whether two TAIs name the same tracking area. models.Tai carries Nid as a
// pointer, so == compares pointer identity for it: two TAIs with equal values but built from
// separate copies never match, and in an SNPN deployment every location update then looked
// like a change and asked the policy function for a fresh decision.
func sameTai(a, b models.Tai) bool {
	if a.PlmnId != b.PlmnId || a.Tac != b.Tac {
		return false
	}

	switch {
	case a.Nid == nil && b.Nid == nil:
		return true
	case a.Nid == nil || b.Nid == nil:
		return false
	default:
		return *a.Nid == *b.Nid
	}
}

func (ranUe *RanUe) UpdateLocation(userLocationInformation *ngapType.UserLocationInformation) {
	if userLocationInformation == nil {
		return
	}

	amfSelf := AMF_Self()
	curTime := time.Now().UTC()
	switch userLocationInformation.Present {
	case ngapType.UserLocationInformationPresentUserLocationInformationEUTRA:
		locationInfoEUTRA := userLocationInformation.UserLocationInformationEUTRA
		if ranUe.Location.EutraLocation == nil {
			ranUe.Location.EutraLocation = models.NewEutraLocationWithDefaults()
		}

		tAI := locationInfoEUTRA.TAI
		plmnID, err := ngapconv.PlmnIdToModels(tAI.PLMNIdentity)
		if err != nil {
			ranUe.Log.Errorf("decode EUTRA TAI PLMN failed: %+v", err)
			return
		}
		tac := hex.EncodeToString(tAI.TAC.Value)

		ranUe.Location.EutraLocation.Tai.PlmnId = plmnID
		ranUe.Location.EutraLocation.Tai.Tac = tac
		ranUe.Tai = ranUe.Location.EutraLocation.Tai

		eUTRACGI := locationInfoEUTRA.EUTRACGI
		ePlmnID, err := ngapconv.PlmnIdToModels(eUTRACGI.PLMNIdentity)
		if err != nil {
			ranUe.Log.Errorf("decode EUTRA CGI PLMN failed: %+v", err)
			return
		}
		eutraCellID := ngapConvert.BitStringToHex(&eUTRACGI.EUTRACellIdentity.Value)

		ranUe.Location.EutraLocation.Ecgi.PlmnId = ePlmnID
		ranUe.Location.EutraLocation.Ecgi.EutraCellId = eutraCellID
		ranUe.Location.EutraLocation.UeLocationTimestamp = &curTime
		if locationInfoEUTRA.TimeStamp != nil {
			ranUe.Location.EutraLocation.SetAgeOfLocationInformation(ngapConvert.TimeStampToInt32(
				locationInfoEUTRA.TimeStamp.Value))
		}
		if ranUe.AmfUe != nil {
			// LocationChanged stays a direct write: its only readers are in
			// HandleMobilityAndPeriodicRegistrationUpdating, on this UE's own
			// goroutine, so unlike Location and Tai it has no cross-goroutine reader
			// to guard against.
			if !sameTai(ranUe.AmfUe.GetTai(), ranUe.Tai) {
				ranUe.AmfUe.LocationChanged = true
			}
			ranUe.AmfUe.SetLocation(ranUe.Location)
			ranUe.AmfUe.SetTai(ranUe.Location.EutraLocation.Tai)
		}
	case ngapType.UserLocationInformationPresentUserLocationInformationNR:
		locationInfoNR := userLocationInformation.UserLocationInformationNR
		if ranUe.Location.NrLocation == nil {
			ranUe.Location.NrLocation = models.NewNrLocationWithDefaults()
		}

		tAI := locationInfoNR.TAI
		plmnID, err := ngapconv.PlmnIdToModels(tAI.PLMNIdentity)
		if err != nil {
			ranUe.Log.Errorf("decode NR TAI PLMN failed: %+v", err)
			return
		}
		tac := hex.EncodeToString(tAI.TAC.Value)

		ranUe.Location.NrLocation.Tai.PlmnId = plmnID
		ranUe.Location.NrLocation.Tai.Tac = tac
		tai := ranUe.Location.NrLocation.GetTai()
		taiCopy := models.NewTai(tai.GetPlmnId(), tai.GetTac())
		if tai.HasNid() {
			taiCopy.SetNid(tai.GetNid())
		}
		ranUe.Tai = *taiCopy

		nRCGI := locationInfoNR.NRCGI
		nRPlmnID, err := ngapconv.PlmnIdToModels(nRCGI.PLMNIdentity)
		if err != nil {
			ranUe.Log.Errorf("decode NR CGI PLMN failed: %+v", err)
			return
		}
		nRCellID := ngapConvert.BitStringToHex(&nRCGI.NRCellIdentity.Value)

		ranUe.Location.NrLocation.Ncgi.SetPlmnId(nRPlmnID)
		ranUe.Location.NrLocation.Ncgi.SetNrCellId(nRCellID)
		ranUe.Location.NrLocation.SetUeLocationTimestamp(curTime)
		if locationInfoNR.TimeStamp != nil {
			ranUe.Location.NrLocation.SetAgeOfLocationInformation(ngapConvert.TimeStampToInt32(locationInfoNR.TimeStamp.Value))
		}
		if ranUe.AmfUe != nil {
			if !sameTai(ranUe.AmfUe.GetTai(), ranUe.Tai) {
				ranUe.AmfUe.LocationChanged = true
			}
			ranUe.AmfUe.SetLocation(ranUe.Location)
			ranUe.AmfUe.SetTai(ranUe.Location.NrLocation.Tai)
		}
	case ngapType.UserLocationInformationPresentUserLocationInformationN3IWF:
		locationInfoN3IWF := userLocationInformation.UserLocationInformationN3IWF
		if ranUe.Location.N3gaLocation == nil {
			ranUe.Location.N3gaLocation = models.NewN3gaLocation()
		}

		ipv4Addr, ipv6Addr := ngapConvert.IPAddressToString(locationInfoN3IWF.IPAddress)

		ranUe.Location.N3gaLocation.UeIpv4Addr = nil
		if ipv4Addr != "" {
			ranUe.Location.N3gaLocation.SetUeIpv4Addr(ipv4Addr)
		}
		ranUe.Location.N3gaLocation.UeIpv6Addr = nil
		if ipv6Addr != "" {
			ranUe.Location.N3gaLocation.SetUeIpv6Addr(ipv6Addr)
		}
		ranUe.Location.N3gaLocation.SetPortNumber(ngapConvert.PortNumberToInt(locationInfoN3IWF.PortNumber))
		// N3GPP TAI is operator-specific
		// TODO: define N3GPP TAI
		tmp, err := strconv.ParseUint(amfSelf.SupportTaiLists[0].GetTac(), 10, 32)
		if err != nil {
			logger.ContextLog.Errorf("error parsing TAC: %v", err)
		}
		tac := fmt.Sprintf("%06x", tmp)
		ranUe.Location.N3gaLocation.N3gppTai = models.NewTai(amfSelf.SupportTaiLists[0].GetPlmnId(), tac)
		tai := ranUe.Location.N3gaLocation.GetN3gppTai()
		taiCopy := models.NewTai(tai.GetPlmnId(), tai.GetTac())
		if tai.HasNid() {
			taiCopy.SetNid(tai.GetNid())
		}
		ranUe.Tai = *taiCopy

		if ranUe.AmfUe != nil {
			ranUe.AmfUe.SetLocation(ranUe.Location)
			ranUe.AmfUe.SetTai(ranUe.Location.N3gaLocation.GetN3gppTai())
		}
	case ngapType.UserLocationInformationPresentNothing:
	}
}
