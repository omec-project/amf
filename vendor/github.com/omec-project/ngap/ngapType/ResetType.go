// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	ResetTypePresentNothing int = iota /* No components present */
	ResetTypePresentNGInterface
	ResetTypePresentPartOfNGInterface
	ResetTypePresentChoiceExtensions
)

type ResetType struct {
	Present           int
	NGInterface       *ResetAll
	PartOfNGInterface *UEAssociatedLogicalNGConnectionList
	ChoiceExtensions  *ProtocolIESingleContainerResetTypeExtIEs
}
