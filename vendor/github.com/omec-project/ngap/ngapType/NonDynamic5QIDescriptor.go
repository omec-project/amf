// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type NonDynamic5QIDescriptor struct {
	FiveQI                 FiveQI
	PriorityLevelQos       *PriorityLevelQos                                        `aper:"optional"`
	AveragingWindow        *AveragingWindow                                         `aper:"optional"`
	MaximumDataBurstVolume *MaximumDataBurstVolume                                  `aper:"optional"`
	IEExtensions           *ProtocolExtensionContainerNonDynamic5QIDescriptorExtIEs `aper:"optional"`
}
