// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type DataForwardingResponseDRBItem struct {
	DRBID                        DRBID
	DLForwardingUPTNLInformation *UPTransportLayerInformation                                   `aper:"valueLB:0,valueUB:1,optional"`
	ULForwardingUPTNLInformation *UPTransportLayerInformation                                   `aper:"valueLB:0,valueUB:1,optional"`
	IEExtensions                 *ProtocolExtensionContainerDataForwardingResponseDRBItemExtIEs `aper:"optional"`
}
