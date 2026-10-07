// SPDX-FileCopyrightText: 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

// Package ngapconv translates between OpenAPI models and NGAP protocol types.
package ngapconv

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/omec-project/ngap/v2/aper"
	"github.com/omec-project/ngap/v2/logger"
	"github.com/omec-project/ngap/v2/ngapConvert"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func AllowedNssaiToNgap(allowed []models.AllowedSnssai) ngapType.AllowedNSSAI {
	result := ngapType.AllowedNSSAI{}
	for _, snssai := range allowed {
		result.List = append(result.List, ngapType.AllowedNSSAIItem{SNSSAI: SNssaiToNgap(snssai.AllowedSnssai)})
	}
	return result
}

func PlmnIdToModels(plmn ngapType.PLMNIdentity) (models.PlmnId, error) {
	if len(plmn.Value) != 3 {
		return models.PlmnId{}, fmt.Errorf("invalid PLMNIdentity length %d", len(plmn.Value))
	}
	encoded := hex.EncodeToString(plmn.Value)
	if len(encoded) != 6 {
		return models.PlmnId{}, fmt.Errorf("invalid encoded PLMNIdentity length %d", len(encoded))
	}
	mcc := string([]byte{encoded[1], encoded[0], encoded[3]})
	if !digits(mcc) {
		return models.PlmnId{}, fmt.Errorf("invalid MCC in PLMNIdentity %q", encoded)
	}
	if encoded[2] == 'f' {
		mnc := string([]byte{encoded[5], encoded[4]})
		if !digits(mnc) {
			return models.PlmnId{}, fmt.Errorf("invalid two-digit MNC in PLMNIdentity %q", encoded)
		}
		return models.PlmnId{Mcc: mcc, Mnc: mnc}, nil
	}
	mnc := string([]byte{encoded[2], encoded[5], encoded[4]})
	if !digits(mnc) {
		return models.PlmnId{}, fmt.Errorf("invalid three-digit MNC in PLMNIdentity %q", encoded)
	}
	return models.PlmnId{Mcc: mcc, Mnc: mnc}, nil
}

func digits(value string) bool {
	for _, digit := range value {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func PlmnIdToNgap(plmn models.PlmnId) ngapType.PLMNIdentity {
	mcc, mnc := strings.Split(plmn.Mcc, ""), strings.Split(plmn.Mnc, "")
	encoded := mcc[1] + mcc[0]
	if len(plmn.Mnc) == 2 {
		encoded += "f" + mcc[2] + mnc[1] + mnc[0]
	} else {
		encoded += mnc[0] + mcc[2] + mnc[2] + mnc[1]
	}
	result := ngapType.PLMNIdentity{}
	if value, err := hex.DecodeString(encoded); err != nil {
		logger.NgapLog.Warnf("decode plmn failed: %+v", err)
	} else {
		result.Value = value
	}
	return result
}

func SNssaiToModels(snssai ngapType.SNSSAI) (models.Snssai, error) {
	if len(snssai.SST.Value) != 1 {
		return models.Snssai{}, fmt.Errorf("invalid S-NSSAI SST length %d", len(snssai.SST.Value))
	}
	result := models.Snssai{Sst: int32(snssai.SST.Value[0])}
	if snssai.SD != nil {
		if len(snssai.SD.Value) != 3 {
			return models.Snssai{}, fmt.Errorf("invalid S-NSSAI SD length %d", len(snssai.SD.Value))
		}
		result.Sd = openapi.PtrString(hex.EncodeToString(snssai.SD.Value))
	}
	return result, nil
}

func SNssaiToNgap(snssai models.Snssai) ngapType.SNSSAI {
	result := ngapType.SNSSAI{}
	result.SST.Value = []byte{byte(snssai.GetSst())}
	if snssai.GetSd() != "" {
		result.SD = new(ngapType.SD)
		if value, err := hex.DecodeString(snssai.GetSd()); err != nil {
			logger.NgapLog.Warnf("decode snssai.sd failed: %+v", err)
		} else {
			result.SD.Value = value
		}
	}
	return result
}

func TaiToModels(tai ngapType.TAI) (models.Tai, error) {
	plmn, err := PlmnIdToModels(tai.PLMNIdentity)
	if err != nil {
		return models.Tai{}, fmt.Errorf("invalid TAI PLMN identity: %w", err)
	}
	return models.Tai{PlmnId: plmn, Tac: hex.EncodeToString(tai.TAC.Value)}, nil
}

func RATRestrictionInformationToNgap(ratType models.RatType) ngapType.RATRestrictionInformation {
	result := ngapType.RATRestrictionInformation{}
	switch ratType {
	case models.RATTYPE_EUTRA:
		result.Value = aper.BitString{Bytes: []byte{0x80}, BitLength: 8}
	case models.RATTYPE_NR:
		result.Value = aper.BitString{Bytes: []byte{0x40}, BitLength: 8}
	}
	return result
}

func RanIdToModels(ran ngapType.GlobalRANNodeID) (models.GlobalRanNodeId, error) {
	result := models.GlobalRanNodeId{}
	switch ran.Present {
	case ngapType.GlobalRANNodeIDPresentGlobalGNBID:
		plmn, err := PlmnIdToModels(ran.GlobalGNBID.PLMNIdentity)
		if err != nil {
			return result, fmt.Errorf("invalid GlobalGNBID PLMN identity: %w", err)
		}
		result.PlmnId, result.GNbId = plmn, models.NewGNbIdWithDefaults()
		if ran.GlobalGNBID.GNBID.Present == ngapType.GNBIDPresentGNBID {
			value := ran.GlobalGNBID.GNBID.GNBID
			result.GNbId.BitLength, result.GNbId.GNBValue = int32(value.BitLength), ngapConvert.BitStringToHex(value)
		}
	case ngapType.GlobalRANNodeIDPresentGlobalNgENBID:
		plmn, err := PlmnIdToModels(ran.GlobalNgENBID.PLMNIdentity)
		if err != nil {
			return result, fmt.Errorf("invalid GlobalNgENBID PLMN identity: %w", err)
		}
		result.PlmnId = plmn
		id := ran.GlobalNgENBID.NgENBID
		switch id.Present {
		case ngapType.NgENBIDPresentMacroNgENBID:
			result.NgeNbId = openapi.PtrString("MacroNGeNB-" + ngapConvert.BitStringToHex(id.MacroNgENBID))
		case ngapType.NgENBIDPresentShortMacroNgENBID:
			result.NgeNbId = openapi.PtrString("SMacroNGeNB-" + ngapConvert.BitStringToHex(id.ShortMacroNgENBID))
		case ngapType.NgENBIDPresentLongMacroNgENBID:
			result.NgeNbId = openapi.PtrString("LMacroNGeNB-" + ngapConvert.BitStringToHex(id.LongMacroNgENBID))
		default:
			return models.GlobalRanNodeId{}, fmt.Errorf("unsupported NgENBID present type %d", id.Present)
		}
	case ngapType.GlobalRANNodeIDPresentGlobalN3IWFID:
		plmn, err := PlmnIdToModels(ran.GlobalN3IWFID.PLMNIdentity)
		if err != nil {
			return result, fmt.Errorf("invalid GlobalN3IWFID PLMN identity: %w", err)
		}
		result.PlmnId = plmn
		if ran.GlobalN3IWFID.N3IWFID.Present == ngapType.N3IWFIDPresentN3IWFID {
			result.N3IwfId = openapi.PtrString(ngapConvert.BitStringToHex(ran.GlobalN3IWFID.N3IWFID.N3IWFID))
		}
	default:
		return models.GlobalRanNodeId{}, fmt.Errorf("unsupported GlobalRANNodeID present type %d", ran.Present)
	}
	return result, nil
}

func TraceDataToNgap(traceData models.TraceData, trsr string) ngapType.TraceActivation {
	result := ngapType.TraceActivation{}
	if len(trsr) != 4 {
		logger.NgapLog.Warnln("trace Recording Session Reference should be 2 octets")
		return result
	}
	parts := strings.Split(traceData.TraceRef, "-")
	if len(parts) != 2 || len(parts[0]) < 5 {
		logger.NgapLog.Warnln("traceRef format is not correct")
		return result
	}
	traceID, err := hex.DecodeString(parts[1])
	if err != nil {
		logger.NgapLog.Warnf("traceIDTmp is empty")
	}
	plmn := models.PlmnId{Mcc: parts[0][:3], Mnc: parts[0][3:]}
	traceRef := append(PlmnIdToNgap(plmn).Value, traceID...)
	trsrValue, err := hex.DecodeString(trsr)
	if err != nil {
		logger.NgapLog.Warnf("decode trsr failed: %+v", err)
	}
	result.NGRANTraceID.Value = append(traceRef, trsrValue...)
	result.InterfacesToTrace.Value = aper.BitString{Bytes: []byte{0}, BitLength: 8}
	if value := traceData.GetInterfaceList(); value != "" {
		if decoded, err := hex.DecodeString(value); err != nil {
			logger.NgapLog.Warnf("decode Interface failed: %+v", err)
		} else if len(decoded) == 1 {
			result.InterfacesToTrace.Value = aper.BitString{Bytes: decoded, BitLength: 8}
		}
	}
	result.TraceCollectionEntityIPAddress = ngapConvert.IPAddressToNgap(traceData.GetCollectionEntityIpv4Addr(), traceData.GetCollectionEntityIpv6Addr())
	switch traceData.GetTraceDepth() {
	case models.TRACEDEPTH_MINIMUM:
		result.TraceDepth.Value = ngapType.TraceDepthPresentMinimum
	case models.TRACEDEPTH_MEDIUM:
		result.TraceDepth.Value = ngapType.TraceDepthPresentMedium
	case models.TRACEDEPTH_MAXIMUM:
		result.TraceDepth.Value = ngapType.TraceDepthPresentMaximum
	case models.TRACEDEPTH_MINIMUM_WO_VENDOR_EXTENSION:
		result.TraceDepth.Value = ngapType.TraceDepthPresentMinimumWithoutVendorSpecificExtension
	case models.TRACEDEPTH_MEDIUM_WO_VENDOR_EXTENSION:
		result.TraceDepth.Value = ngapType.TraceDepthPresentMediumWithoutVendorSpecificExtension
	case models.TRACEDEPTH_MAXIMUM_WO_VENDOR_EXTENSION:
		result.TraceDepth.Value = ngapType.TraceDepthPresentMaximumWithoutVendorSpecificExtension
	}
	return result
}
