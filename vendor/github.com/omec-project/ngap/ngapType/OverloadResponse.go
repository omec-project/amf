// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	OverloadResponsePresentNothing int = iota /* No components present */
	OverloadResponsePresentOverloadAction
	OverloadResponsePresentChoiceExtensions
)

type OverloadResponse struct {
	Present          int
	OverloadAction   *OverloadAction
	ChoiceExtensions *ProtocolIESingleContainerOverloadResponseExtIEs
}
