// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	NGRANCGIPresentNothing int = iota /* No components present */
	NGRANCGIPresentNRCGI
	NGRANCGIPresentEUTRACGI
	NGRANCGIPresentChoiceExtensions
)

type NGRANCGI struct {
	Present          int
	NRCGI            *NRCGI    `aper:"valueExt"`
	EUTRACGI         *EUTRACGI `aper:"valueExt"`
	ChoiceExtensions *ProtocolIESingleContainerNGRANCGIExtIEs
}
