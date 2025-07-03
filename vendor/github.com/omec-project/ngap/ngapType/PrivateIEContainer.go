// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

/* Sequence of = 35, FULL Name = struct PrivateIE_Container_6722P0 */
/* PrivateMessageIEs */
type PrivateIEContainerPrivateMessageIEs struct {
	List []PrivateMessageIEs `aper:"sizeLB:1,sizeUB:65535"`
}
