// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	PrivateIEIDPresentNothing int = iota /* No components present */
	PrivateIEIDPresentLocal
	PrivateIEIDPresentGlobal
)

type PrivateIEID struct {
	Present int
	Local   *int64 `aper:"valueLB:0,valueUB:65535"`
	Global  *aper.ObjectIdentifier
}
