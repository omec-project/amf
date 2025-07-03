// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type QosFlowLevelQosParameters struct {
	QosCharacteristics             QosCharacteristics                                         `aper:"valueLB:0,valueUB:2"`
	AllocationAndRetentionPriority AllocationAndRetentionPriority                             `aper:"valueExt"`
	GBRQosInformation              *GBRQosInformation                                         `aper:"valueExt,optional"`
	ReflectiveQosAttribute         *ReflectiveQosAttribute                                    `aper:"optional"`
	AdditionalQosFlowInformation   *AdditionalQosFlowInformation                              `aper:"optional"`
	IEExtensions                   *ProtocolExtensionContainerQosFlowLevelQosParametersExtIEs `aper:"optional"`
}
