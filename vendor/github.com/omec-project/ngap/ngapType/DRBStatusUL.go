// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	DRBStatusULPresentNothing int = iota /* No components present */
	DRBStatusULPresentDRBStatusUL12
	DRBStatusULPresentDRBStatusUL18
	DRBStatusULPresentChoiceExtensions
)

type DRBStatusUL struct {
	Present          int
	DRBStatusUL12    *DRBStatusUL12 `aper:"valueExt"`
	DRBStatusUL18    *DRBStatusUL18 `aper:"valueExt"`
	ChoiceExtensions *ProtocolIESingleContainerDRBStatusULExtIEs
}
