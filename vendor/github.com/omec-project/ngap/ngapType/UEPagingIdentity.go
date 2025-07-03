// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	UEPagingIdentityPresentNothing int = iota /* No components present */
	UEPagingIdentityPresentFiveGSTMSI
	UEPagingIdentityPresentChoiceExtensions
)

type UEPagingIdentity struct {
	Present          int
	FiveGSTMSI       *FiveGSTMSI `aper:"valueExt"`
	ChoiceExtensions *ProtocolIESingleContainerUEPagingIdentityExtIEs
}
