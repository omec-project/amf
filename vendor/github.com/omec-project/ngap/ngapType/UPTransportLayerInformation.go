// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	UPTransportLayerInformationPresentNothing int = iota /* No components present */
	UPTransportLayerInformationPresentGTPTunnel
	UPTransportLayerInformationPresentChoiceExtensions
)

type UPTransportLayerInformation struct {
	Present          int
	GTPTunnel        *GTPTunnel `aper:"valueExt"`
	ChoiceExtensions *ProtocolIESingleContainerUPTransportLayerInformationExtIEs
}
