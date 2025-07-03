// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type PDUSessionResourceSetupResponseTransfer struct {
	DLQosFlowPerTNLInformation           QosFlowPerTNLInformation                                                 `aper:"valueExt"`
	AdditionalDLQosFlowPerTNLInformation *QosFlowPerTNLInformationList                                            `aper:"optional"`
	SecurityResult                       *SecurityResult                                                          `aper:"valueExt,optional"`
	QosFlowFailedToSetupList             *QosFlowListWithCause                                                    `aper:"optional"`
	IEExtensions                         *ProtocolExtensionContainerPDUSessionResourceSetupResponseTransferExtIEs `aper:"optional"`
}
