// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package context

// NtnAccessInfo captures Non-Terrestrial Network access properties for a
// UE, as derived from NGAP signaling. Only NTN-access presence is tracked
// today; future 3GPP-defined fields (satelliteBackhaulCategory,
// geoSatelliteId, NTN TAI list) can be added without reshaping AmfUe or
// RanUe.
type NtnAccessInfo struct {
	// Detected is set true the first time the NRNTN-TAI-Information
	// protocol extension is observed in a UserLocationInformationNR for
	// this UE. It is sticky: once set, it is not cleared while the UE
	// context lives.
	//
	// The extension is PRESENCE optional per 3GPP TS 38.413, and gNBs
	// include it at their discretion — typically on context-establishing
	// events (initial UE message, handover) but often not on routine
	// uplink/location reports. Treating an absent extension as evidence
	// of "not NTN" would cause IsNtn() to flap on routine NGAP traffic
	// for a UE that genuinely is on NTN; sticky-on avoids that.
	Detected bool `json:"detected"`
}

// IsNtn reports whether NTN access has been observed for this UE during
// the current UE context. The flag is sticky for the session — see
// NtnAccessInfo.Detected for the rationale.
func (ue *AmfUe) IsNtn() bool {
	return ue.NtnAccess != nil && ue.NtnAccess.Detected
}

// updateNtnAccess records observed NTN access on this RanUe.
//
// detected==false is a no-op: the NRNTN-TAI-Information extension is
// PRESENCE optional per 3GPP TS 38.413, so an absent extension does not
// imply the UE has left NTN access. The flag is sticky for the lifetime
// of the UE context.
func (ranUe *RanUe) updateNtnAccess(detected bool) {
	if !detected {
		return
	}
	if ranUe.NtnAccess != nil && ranUe.NtnAccess.Detected {
		return
	}
	if ranUe.NtnAccess == nil {
		ranUe.NtnAccess = &NtnAccessInfo{}
	}
	ranUe.NtnAccess.Detected = true
	ranUe.Log.Debugf("NR NTN access detected (NRNTN-TAI-Information extension present)")
}
