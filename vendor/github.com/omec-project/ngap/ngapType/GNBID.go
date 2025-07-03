// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	GNBIDPresentNothing int = iota /* No components present */
	GNBIDPresentGNBID
	GNBIDPresentChoiceExtensions
)

type GNBID struct {
	Present          int
	GNBID            *aper.BitString `aper:"sizeLB:22,sizeUB:32"`
	ChoiceExtensions *ProtocolIESingleContainerGNBIDExtIEs
}
