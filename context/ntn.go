// SPDX-FileCopyrightText: 2026 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package context

// NtnAccessInfo captures Non-Terrestrial Network access properties for a
// UE, as derived from NGAP signaling. Only NTN-access presence is tracked
// today; future 3GPP-defined fields (satelliteBackhaulCategory,
// geoSatelliteId, NTN TAI list) can be added without reshaping AmfUe or
// RanUe.
type NtnAccessInfo struct {
	// Detected reflects whether the most recent NR UserLocationInformation
	// for this UE carried the NRNTN-TAI-Information extension. It is set
	// and cleared on every NR location update.
	Detected bool `json:"detected"`
}

// IsNtn reports whether the UE is currently being served over NTN access,
// based on the most recent NR location update.
func (ue *AmfUe) IsNtn() bool {
	return ue.NtnAccess != nil && ue.NtnAccess.Detected
}

// updateNtnAccess records the current NTN-detection state on this RanUe.
// It allocates the NtnAccessInfo container on first use and logs at Debug
// only when the state actually transitions, to keep the per-update path
// quiet for steady-state UEs.
func (ranUe *RanUe) updateNtnAccess(detected bool) {
	if ranUe.NtnAccess == nil {
		ranUe.NtnAccess = &NtnAccessInfo{}
	}
	if ranUe.NtnAccess.Detected == detected {
		return
	}
	ranUe.NtnAccess.Detected = detected
	if detected {
		ranUe.Log.Debugf("NR NTN access detected (NRNTN-TAI-Information extension present)")
	} else {
		ranUe.Log.Debugf("NR NTN access cleared (no NRNTN-TAI-Information extension)")
	}
}
