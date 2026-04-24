// SPDX-FileCopyrightText: 2026 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package context

import "github.com/omec-project/ngap/ngapType"

// ProtocolExtensionIDNRNTNTAIInformation is the NGAP protocol IE/extension
// identifier assigned to NRNTN-TAI-Information per 3GPP TS 38.413. Its
// presence inside UserLocationInformationNR.IEExtensions indicates that
// the UE is being served over NR Non-Terrestrial Network access.
const ProtocolExtensionIDNRNTNTAIInformation int64 = 287

// NtnAccessInfo captures Non-Terrestrial Network access properties for a
// UE, as derived from NGAP signaling. Only NTN-access presence is tracked
// today; future 3GPP-defined fields (satelliteBackhaulCategory,
// geoSatelliteId, NTN TAI list) can be added without reshaping AmfUe or
// RanUe.
type NtnAccessInfo struct {
	// Detected is true when NTN access has been observed for this UE.
	Detected bool `json:"detected"`
}

// IsNtn reports whether the UE is being served over NTN access.
func (ue *AmfUe) IsNtn() bool {
	return ue.NtnAccess != nil && ue.NtnAccess.Detected
}

// nrLocationHasNtnExt reports whether the given UserLocationInformationNR
// carries the NRNTN-TAI-Information protocol extension.
func nrLocationHasNtnExt(loc *ngapType.UserLocationInformationNR) bool {
	if loc == nil || loc.IEExtensions == nil {
		return false
	}
	for _, ext := range loc.IEExtensions.List {
		if ext.Id.Value == ProtocolExtensionIDNRNTNTAIInformation {
			return true
		}
	}
	return false
}
