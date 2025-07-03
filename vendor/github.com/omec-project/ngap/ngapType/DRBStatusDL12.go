// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type DRBStatusDL12 struct {
	DLCOUNTValue COUNTValueForPDCPSN12                          `aper:"valueExt"`
	IEExtension  *ProtocolExtensionContainerDRBStatusDL12ExtIEs `aper:"optional"`
}
