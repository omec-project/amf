// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type PDUSessionResourceInformationItem struct {
	PDUSessionID              PDUSessionID
	QosFlowInformationList    QosFlowInformationList
	DRBsToQosFlowsMappingList *DRBsToQosFlowsMappingList                                         `aper:"optional"`
	IEExtensions              *ProtocolExtensionContainerPDUSessionResourceInformationItemExtIEs `aper:"optional"`
}
