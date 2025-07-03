// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

type VolumeTimedReportItem struct {
	StartTimeStamp aper.OctetString                                       `aper:"sizeLB:4,sizeUB:4"`
	EndTimeStamp   aper.OctetString                                       `aper:"sizeLB:4,sizeUB:4"`
	UsageCountUL   int64                                                  `aper:"valueLB:0,valueUB:18446744073709551615"`
	UsageCountDL   int64                                                  `aper:"valueLB:0,valueUB:18446744073709551615"`
	IEExtensions   *ProtocolExtensionContainerVolumeTimedReportItemExtIEs `aper:"optional"`
}
