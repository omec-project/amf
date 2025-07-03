// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type HandoverCommandTransfer struct {
	DLForwardingUPTNLInformation  *UPTransportLayerInformation                             `aper:"valueLB:0,valueUB:1,optional"`
	QosFlowToBeForwardedList      *QosFlowToBeForwardedList                                `aper:"optional"`
	DataForwardingResponseDRBList *DataForwardingResponseDRBList                           `aper:"optional"`
	IEExtensions                  *ProtocolExtensionContainerHandoverCommandTransferExtIEs `aper:"optional"`
}
