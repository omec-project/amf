// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type SONConfigurationTransfer struct {
	TargetRANNodeID        TargetRANNodeID                                           `aper:"valueExt"`
	SourceRANNodeID        SourceRANNodeID                                           `aper:"valueExt"`
	SONInformation         SONInformation                                            `aper:"valueLB:0,valueUB:2"`
	XnTNLConfigurationInfo *XnTNLConfigurationInfo                                   `aper:"valueExt,optional"`
	IEExtensions           *ProtocolExtensionContainerSONConfigurationTransferExtIEs `aper:"optional"`
}
