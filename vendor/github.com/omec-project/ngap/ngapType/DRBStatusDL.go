// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	DRBStatusDLPresentNothing int = iota /* No components present */
	DRBStatusDLPresentDRBStatusDL12
	DRBStatusDLPresentDRBStatusDL18
	DRBStatusDLPresentChoiceExtensions
)

type DRBStatusDL struct {
	Present          int
	DRBStatusDL12    *DRBStatusDL12 `aper:"valueExt"`
	DRBStatusDL18    *DRBStatusDL18 `aper:"valueExt"`
	ChoiceExtensions *ProtocolIESingleContainerDRBStatusDLExtIEs
}
