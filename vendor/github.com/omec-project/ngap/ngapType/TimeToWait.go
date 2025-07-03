// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	TimeToWaitPresentV1s  aper.Enumerated = 0
	TimeToWaitPresentV2s  aper.Enumerated = 1
	TimeToWaitPresentV5s  aper.Enumerated = 2
	TimeToWaitPresentV10s aper.Enumerated = 3
	TimeToWaitPresentV20s aper.Enumerated = 4
	TimeToWaitPresentV60s aper.Enumerated = 5
)

type TimeToWait struct {
	Value aper.Enumerated `aper:"valueExt,valueLB:0,valueUB:5"`
}
