// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type COUNTValueForPDCPSN18 struct {
	PDCPSN18     int64                                                  `aper:"valueLB:0,valueUB:262143"`
	HFNPDCPSN18  int64                                                  `aper:"valueLB:0,valueUB:16383"`
	IEExtensions *ProtocolExtensionContainerCOUNTValueForPDCPSN18ExtIEs `aper:"optional"`
}
