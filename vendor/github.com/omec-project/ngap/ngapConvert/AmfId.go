// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapConvert

import (
	"encoding/hex"

	"github.com/omec-project/aper"
	"github.com/omec-project/ngap/logger"
)

func AmfIdToNgap(amfId string) (regionId, setId, ptrId aper.BitString) {
	regionId = HexToBitString(amfId[:2], 8)
	setId = HexToBitString(amfId[2:5], 10)
	tmpByte, err := hex.DecodeString(amfId[4:])
	if err != nil {
		logger.NgapLog.Warnf("amfId From Models To NGAP Error: %v", err)
		return
	}
	shiftByte, err := aper.GetBitString(tmpByte, 2, 6)
	if err != nil {
		logger.NgapLog.Warnf("amfId From Models To NGAP Error: %v", err)
		return
	}
	ptrId.BitLength = 6
	ptrId.Bytes = shiftByte
	return
}

func AmfIdToModels(regionId, setId, ptrId aper.BitString) (amfId string) {
	regionHex := BitStringToHex(&regionId)
	tmpByte := []byte{setId.Bytes[0], (setId.Bytes[1] & 0xc0) | (ptrId.Bytes[0] >> 2)}
	restHex := hex.EncodeToString(tmpByte)
	amfId = regionHex + restHex
	return
}
