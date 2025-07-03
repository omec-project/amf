// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type Dynamic5QIDescriptor struct {
	PriorityLevelQos       PriorityLevelQos
	PacketDelayBudget      PacketDelayBudget
	PacketErrorRate        PacketErrorRate                                       `aper:"valueExt"`
	FiveQI                 *FiveQI                                               `aper:"optional"`
	DelayCritical          *DelayCritical                                        `aper:"optional"`
	AveragingWindow        *AveragingWindow                                      `aper:"optional"`
	MaximumDataBurstVolume *MaximumDataBurstVolume                               `aper:"optional"`
	IEExtensions           *ProtocolExtensionContainerDynamic5QIDescriptorExtIEs `aper:"optional"`
}
