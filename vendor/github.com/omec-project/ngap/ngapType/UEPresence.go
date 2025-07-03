// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	UEPresencePresentIn      aper.Enumerated = 0
	UEPresencePresentOut     aper.Enumerated = 1
	UEPresencePresentUnknown aper.Enumerated = 2
)

type UEPresence struct {
	Value aper.Enumerated `aper:"valueExt,valueLB:0,valueUB:2"`
}
