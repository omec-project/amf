// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type ExpectedUEBehaviour struct {
	ExpectedUEActivityBehaviour *ExpectedUEActivityBehaviour                         `aper:"valueExt,optional"`
	ExpectedHOInterval          *ExpectedHOInterval                                  `aper:"optional"`
	ExpectedUEMobility          *ExpectedUEMobility                                  `aper:"optional"`
	ExpectedUEMovingTrajectory  *ExpectedUEMovingTrajectory                          `aper:"optional"`
	IEExtensions                *ProtocolExtensionContainerExpectedUEBehaviourExtIEs `aper:"optional"`
}
