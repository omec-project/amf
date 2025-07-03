// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	N3IWFIDPresentNothing int = iota /* No components present */
	N3IWFIDPresentN3IWFID
	N3IWFIDPresentChoiceExtensions
)

type N3IWFID struct {
	Present          int
	N3IWFID          *aper.BitString `aper:"sizeLB:16,sizeUB:16"`
	ChoiceExtensions *ProtocolIESingleContainerN3IWFIDExtIEs
}
