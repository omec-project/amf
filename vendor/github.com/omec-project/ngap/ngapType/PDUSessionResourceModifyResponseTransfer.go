// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type PDUSessionResourceModifyResponseTransfer struct {
	DLNGUUPTNLInformation                *UPTransportLayerInformation                                              `aper:"valueLB:0,valueUB:1,optional"`
	ULNGUUPTNLInformation                *UPTransportLayerInformation                                              `aper:"valueLB:0,valueUB:1,optional"`
	QosFlowAddOrModifyResponseList       *QosFlowAddOrModifyResponseList                                           `aper:"optional"`
	AdditionalDLQosFlowPerTNLInformation *QosFlowPerTNLInformationList                                             `aper:"optional"`
	QosFlowFailedToAddOrModifyList       *QosFlowListWithCause                                                     `aper:"optional"`
	IEExtensions                         *ProtocolExtensionContainerPDUSessionResourceModifyResponseTransferExtIEs `aper:"optional"`
}
