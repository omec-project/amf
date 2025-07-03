// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	EmergencyServiceTargetCNPresentFiveGC aper.Enumerated = 0
	EmergencyServiceTargetCNPresentEpc    aper.Enumerated = 1
)

type EmergencyServiceTargetCN struct {
	Value aper.Enumerated `aper:"valueExt,valueLB:0,valueUB:1"`
}
