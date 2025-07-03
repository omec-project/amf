// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type QosFlowSetupRequestItem struct {
	QosFlowIdentifier         QosFlowIdentifier
	QosFlowLevelQosParameters QosFlowLevelQosParameters                                `aper:"valueExt"`
	ERABID                    *ERABID                                                  `aper:"optional"`
	IEExtensions              *ProtocolExtensionContainerQosFlowSetupRequestItemExtIEs `aper:"optional"`
}
