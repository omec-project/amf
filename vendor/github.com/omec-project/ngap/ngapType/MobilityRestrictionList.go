// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type MobilityRestrictionList struct {
	ServingPLMN              PLMNIdentity
	EquivalentPLMNs          *EquivalentPLMNs                                         `aper:"optional"`
	RATRestrictions          *RATRestrictions                                         `aper:"optional"`
	ForbiddenAreaInformation *ForbiddenAreaInformation                                `aper:"optional"`
	ServiceAreaInformation   *ServiceAreaInformation                                  `aper:"optional"`
	IEExtensions             *ProtocolExtensionContainerMobilityRestrictionListExtIEs `aper:"optional"`
}
