// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapConvert

import (
	"encoding/hex"
	"strings"

	"github.com/omec-project/aper"
	"github.com/omec-project/ngap/logger"
	"github.com/omec-project/ngap/ngapType"
	"github.com/omec-project/openapi/models"
)

func TraceDataToModels(traceActivation ngapType.TraceActivation) (traceData models.TraceData) {
	// TODO: finish this function when need
	return
}

func TraceDataToNgap(traceData models.TraceData, trsr string) ngapType.TraceActivation {
	var traceActivation ngapType.TraceActivation

	if len(trsr) != 4 {
		logger.NgapLog.Warnln("trace Recording Session Reference should be 2 octets")
		return traceActivation
	}

	// NG-RAN Trace ID (left most 6 octet Trace Reference + last 2 octet Trace Recoding Session Reference)
	subStringSlice := strings.Split(traceData.TraceRef, "-")

	if len(subStringSlice) != 2 {
		logger.NgapLog.Warnln("traceRef format is not correct")
		return traceActivation
	}

	plmnID := models.PlmnId{}
	plmnID.Mcc = subStringSlice[0][:3]
	plmnID.Mnc = subStringSlice[0][3:]
	var traceID []byte
	if traceIDTmp, err := hex.DecodeString(subStringSlice[1]); err != nil {
		logger.NgapLog.Warnf("traceIDTmp is empty")
	} else {
		traceID = traceIDTmp
	}

	tmp := PlmnIdToNgap(plmnID)
	traceReference := append(tmp.Value, traceID...)
	var trsrNgap []byte
	if trsrNgapTmp, err := hex.DecodeString(trsr); err != nil {
		logger.NgapLog.Warnf("decode trsr failed: %+v", err)
	} else {
		trsrNgap = trsrNgapTmp
	}

	nGRANTraceID := append(traceReference, trsrNgap...)

	traceActivation.NGRANTraceID.Value = nGRANTraceID

	// Interfaces To Trace
	var interfacesToTrace []byte
	if interfacesToTraceTmp, err := hex.DecodeString(traceData.InterfaceList); err != nil {
		logger.NgapLog.Warnf("decode Interface failed: %+v", err)
	} else {
		interfacesToTrace = interfacesToTraceTmp
	}
	traceActivation.InterfacesToTrace.Value = aper.BitString{
		Bytes:     interfacesToTrace,
		BitLength: 8,
	}

	// Trace Collection Entity IP Address
	ngapIP := IPAddressToNgap(traceData.CollectionEntityIpv4Addr, traceData.CollectionEntityIpv6Addr)
	traceActivation.TraceCollectionEntityIPAddress = ngapIP

	// Trace Depth
	switch traceData.TraceDepth {
	case models.TraceDepth_MINIMUM:
		traceActivation.TraceDepth.Value = ngapType.TraceDepthPresentMinimum
	case models.TraceDepth_MEDIUM:
		traceActivation.TraceDepth.Value = ngapType.TraceDepthPresentMedium
	case models.TraceDepth_MAXIMUM:
		traceActivation.TraceDepth.Value = ngapType.TraceDepthPresentMaximum
	case models.TraceDepth_MINIMUM_WO_VENDOR_EXTENSION:
		traceActivation.TraceDepth.Value = ngapType.TraceDepthPresentMinimumWithoutVendorSpecificExtension
	case models.TraceDepth_MEDIUM_WO_VENDOR_EXTENSION:
		traceActivation.TraceDepth.Value = ngapType.TraceDepthPresentMediumWithoutVendorSpecificExtension
	case models.TraceDepth_MAXIMUM_WO_VENDOR_EXTENSION:
		traceActivation.TraceDepth.Value = ngapType.TraceDepthPresentMaximumWithoutVendorSpecificExtension
	}

	return traceActivation
}
