// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type SourceNGRANNodeToTargetNGRANNodeTransparentContainer struct {
	RRCContainer                      RRCContainer
	PDUSessionResourceInformationList *PDUSessionResourceInformationList `aper:"optional"`
	ERABInformationList               *ERABInformationList               `aper:"optional"`
	TargetCellID                      NGRANCGI                           `aper:"valueLB:0,valueUB:2"`
	IndexToRFSP                       *IndexToRFSP                       `aper:"optional"`
	UEHistoryInformation              UEHistoryInformation
	IEExtensions                      *ProtocolExtensionContainerSourceNGRANNodeToTargetNGRANNodeTransparentContainerExtIEs `aper:"optional"`
}
