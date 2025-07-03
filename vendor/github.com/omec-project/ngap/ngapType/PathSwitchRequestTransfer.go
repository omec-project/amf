// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type PathSwitchRequestTransfer struct {
	DLNGUUPTNLInformation        UPTransportLayerInformation   `aper:"valueLB:0,valueUB:1"`
	DLNGUTNLInformationReused    *DLNGUTNLInformationReused    `aper:"optional"`
	UserPlaneSecurityInformation *UserPlaneSecurityInformation `aper:"valueExt,optional"`
	QosFlowAcceptedList          QosFlowAcceptedList
	IEExtensions                 *ProtocolExtensionContainerPathSwitchRequestTransferExtIEs `aper:"optional"`
}
