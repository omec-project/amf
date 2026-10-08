// SPDX-FileCopyrightText: 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package nasconv

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"strconv"

	"github.com/omec-project/nas/v2/logger"
	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/nas/v2/nasType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

// RequestedNssaiToModels converts a NAS Requested NSSAI IE to its SBI model.
func RequestedNssaiToModels(nasNssai *nasType.RequestedNSSAI) ([]models.MappingOfSnssai, error) {
	buf := nasNssai.GetSNSSAIValue()
	length := int(nasNssai.GetLen())
	if length > len(buf) {
		return nil, fmt.Errorf("requested NSSAI length %d exceeds buffer length %d", length, len(buf))
	}
	buf = buf[:length]

	requested := make([]models.MappingOfSnssai, 0)
	for offset := 0; offset < length; {
		itemLength := buf[offset]
		item, err := requestedSnssaiToModels(itemLength, buf[offset:])
		if err != nil {
			return nil, err
		}
		requested = append(requested, item)
		offset += int(itemLength) + 1
	}
	return requested, nil
}

func requestedSnssaiToModels(length uint8, buf []byte) (models.MappingOfSnssai, error) {
	result := models.MappingOfSnssai{}
	if int(length)+1 > len(buf) {
		return result, fmt.Errorf("S-NSSAI contents length %d exceeds remaining buffer", length)
	}
	withSD := func(sst byte, sd []byte) models.Snssai {
		return models.Snssai{Sst: int32(sst), Sd: openapi.PtrString(hex.EncodeToString(sd))}
	}
	switch length {
	case 0x01:
		result.ServingSnssai = models.Snssai{Sst: int32(buf[1])}
	case 0x02:
		result.ServingSnssai = models.Snssai{Sst: int32(buf[1])}
		result.HomeSnssai = models.Snssai{Sst: int32(buf[2])}
	case 0x04:
		result.ServingSnssai = withSD(buf[1], buf[2:5])
	case 0x05:
		result.ServingSnssai = withSD(buf[1], buf[2:5])
		result.HomeSnssai = models.Snssai{Sst: int32(buf[5])}
	case 0x08:
		result.ServingSnssai = withSD(buf[1], buf[2:5])
		result.HomeSnssai = withSD(buf[5], buf[6:9])
	default:
		return result, fmt.Errorf("invalid length of S-NSSAI contents: %d", length)
	}
	return result, nil
}

func SnssaiToModels(nasSnssai *nasType.SNSSAI) models.Snssai {
	sd := nasSnssai.GetSD()
	return models.Snssai{Sst: int32(nasSnssai.GetSST()), Sd: openapi.PtrString(hex.EncodeToString(sd[:]))}
}

func SnssaiToNas(snssai models.Snssai) []uint8 {
	if snssai.GetSd() == "" {
		return []uint8{0x01, uint8(snssai.Sst)}
	}
	result := []uint8{0x04, uint8(snssai.Sst)}
	if sd, err := hex.DecodeString(snssai.GetSd()); err != nil {
		logger.ConvertLog.Warnf("decode snssai.sd failed: %+v", err)
	} else {
		result = append(result, sd...)
	}
	return result
}

func rejectedSnssaiToNas(snssai models.Snssai, cause uint8) []uint8 {
	if snssai.GetSd() == "" {
		return []uint8{(0x01 << 4) + cause, uint8(snssai.Sst)}
	}
	result := []uint8{(0x04 << 4) + cause, uint8(snssai.Sst)}
	if sd, err := hex.DecodeString(snssai.GetSd()); err != nil {
		logger.ConvertLog.Warnf("decode snssai.sd failed: %+v", err)
	} else {
		result = append(result, sd...)
	}
	return result
}

func RejectedNssaiToNas(inPlmn, inTai []models.Snssai) nasType.RejectedNSSAI {
	contents := make([]uint8, 0)
	for _, snssai := range inPlmn {
		contents = append(contents, rejectedSnssaiToNas(snssai, nasMessage.RejectedSnssaiCauseNotAvailableInCurrentPlmn)...)
	}
	for _, snssai := range inTai {
		contents = append(contents, rejectedSnssaiToNas(snssai, nasMessage.RejectedSnssaiCauseNotAvailableInCurrentRegistrationArea)...)
	}
	result := nasType.RejectedNSSAI{}
	result.SetLen(uint8(len(contents)))
	result.SetRejectedNSSAIContents(contents)
	return result
}

func PlmnIDToNas(plmnID models.PlmnId) []uint8 {
	digit := func(value string, index int) int {
		if parsed, err := strconv.Atoi(string(value[index])); err == nil {
			return parsed
		}
		logger.ConvertLog.Warnf("invalid PLMN digit in %q", value)
		return 0
	}
	mcc1, mcc2, mcc3 := digit(plmnID.Mcc, 0), digit(plmnID.Mcc, 1), digit(plmnID.Mcc, 2)
	mnc1, mnc2, mnc3 := digit(plmnID.Mnc, 0), digit(plmnID.Mnc, 1), 0x0f
	if len(plmnID.Mnc) == 3 {
		mnc3 = digit(plmnID.Mnc, 2)
	}
	return []uint8{uint8((mcc2 << 4) | mcc1), uint8((mnc3 << 4) | mcc3), uint8((mnc2 << 4) | mnc1)}
}

func TaiListToNas(taiList []models.Tai) []uint8 {
	typeOfList := 0x00
	plmnID := taiList[0].PlmnId
	for _, tai := range taiList {
		if !reflect.DeepEqual(plmnID, tai.PlmnId) {
			typeOfList = 0x02
		}
	}
	result := []uint8{uint8(typeOfList<<5) + uint8(len(taiList)-1)}
	if typeOfList == 0x00 {
		result = append(result, PlmnIDToNas(plmnID)...)
		for _, tai := range taiList {
			if tac, err := hex.DecodeString(tai.Tac); err != nil {
				logger.ConvertLog.Warnf("decode tac failed: %+v", err)
			} else {
				result = append(result, tac...)
			}
		}
		return result
	}
	for _, tai := range taiList {
		if tac, err := hex.DecodeString(tai.Tac); err != nil {
			logger.ConvertLog.Warnf("decode tac failed: %+v", err)
		} else {
			result = append(result, PlmnIDToNas(tai.PlmnId)...)
			result = append(result, tac...)
		}
	}
	return result
}

func LadnToNas(dnn string, taiLists []models.Tai) []uint8 {
	result := append([]uint8{uint8(len(dnn))}, []byte(dnn)...)
	taiList := TaiListToNas(taiLists)
	result = append(result, uint8(len(taiList)))
	return append(result, taiList...)
}

func PartialServiceAreaListToNas(plmnID models.PlmnId, restriction models.ServiceAreaRestriction) []byte {
	allowedType := nasMessage.AllowedTypeNonAllowedArea
	if restriction.RestrictionType != nil && *restriction.RestrictionType == models.RESTRICTIONTYPE_ALLOWED_AREAS {
		allowedType = nasMessage.AllowedTypeAllowedArea
	}
	result := []byte{((allowedType << 7) & 0x80) + uint8(len(restriction.Areas))}
	result = append(result, PlmnIDToNas(plmnID)...)
	for _, area := range restriction.Areas {
		for _, tac := range area.Tacs {
			if bytes, err := hex.DecodeString(tac); err != nil {
				logger.ConvertLog.Warnf("decode tac failed: %+v", err)
			} else {
				result = append(result, bytes...)
			}
		}
	}
	return result
}

func GutiToString(buf []byte) (models.Guami, string) {
	if len(buf) != 11 {
		logger.ConvertLog.Errorf("invalid GUTI buffer length: %d", len(buf))
		return models.Guami{}, ""
	}
	plmnID := plmnIDToString(buf[1:4])
	guami := models.Guami{PlmnId: models.PlmnIdNid{Mcc: plmnID[:3], Mnc: plmnID[3:]}, AmfId: hex.EncodeToString(buf[4:7])}
	return guami, plmnID + guami.AmfId + hex.EncodeToString(buf[7:])
}

func plmnIDToString(buf []byte) string {
	if len(buf) < 3 {
		logger.ConvertLog.Errorf("invalid PLMN ID buffer length: %d", len(buf))
		return ""
	}
	value := hex.EncodeToString([]byte{((buf[0] & 0x0f) << 4) | (buf[0] >> 4), ((buf[1] & 0x0f) << 4) | (buf[2] & 0x0f), ((buf[2] & 0xf0) >> 4 << 4) | ((buf[1] & 0xf0) >> 4)})
	if value[5] == 'f' {
		return value[:5]
	}
	return value
}

func SpareHalfOctetAndNgksiToNas(ngKsi models.NgKsi) nasType.SpareHalfOctetAndNgksi {
	result := nasType.SpareHalfOctetAndNgksi{}
	switch ngKsi.Tsc {
	case models.SCTYPE_NATIVE:
		result.SetTSC(nasMessage.TypeOfSecurityContextFlagNative)
	case models.SCTYPE_MAPPED:
		result.SetTSC(nasMessage.TypeOfSecurityContextFlagMapped)
	}
	result.SetSpareHalfOctet(0)
	result.SetNasKeySetIdentifiler(uint8(ngKsi.Ksi))
	return result
}
