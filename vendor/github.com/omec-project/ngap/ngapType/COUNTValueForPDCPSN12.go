// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type COUNTValueForPDCPSN12 struct {
	PDCPSN12     int64                                                  `aper:"valueLB:0,valueUB:4095"`
	HFNPDCPSN12  int64                                                  `aper:"valueLB:0,valueUB:1048575"`
	IEExtensions *ProtocolExtensionContainerCOUNTValueForPDCPSN12ExtIEs `aper:"optional"`
}
