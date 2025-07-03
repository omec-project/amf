// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	GlobalRANNodeIDPresentNothing int = iota /* No components present */
	GlobalRANNodeIDPresentGlobalGNBID
	GlobalRANNodeIDPresentGlobalNgENBID
	GlobalRANNodeIDPresentGlobalN3IWFID
	GlobalRANNodeIDPresentChoiceExtensions
)

type GlobalRANNodeID struct {
	Present          int
	GlobalGNBID      *GlobalGNBID   `aper:"valueExt"`
	GlobalNgENBID    *GlobalNgENBID `aper:"valueExt"`
	GlobalN3IWFID    *GlobalN3IWFID `aper:"valueExt"`
	ChoiceExtensions *ProtocolIESingleContainerGlobalRANNodeIDExtIEs
}
