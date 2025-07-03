// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	HandoverFlagPresentHandoverPreparation aper.Enumerated = 0
)

type HandoverFlag struct {
	Value aper.Enumerated `aper:"valueExt,valueLB:0,valueUB:0"`
}
