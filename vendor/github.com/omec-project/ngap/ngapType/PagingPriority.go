// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	PagingPriorityPresentPriolevel1 aper.Enumerated = 0
	PagingPriorityPresentPriolevel2 aper.Enumerated = 1
	PagingPriorityPresentPriolevel3 aper.Enumerated = 2
	PagingPriorityPresentPriolevel4 aper.Enumerated = 3
	PagingPriorityPresentPriolevel5 aper.Enumerated = 4
	PagingPriorityPresentPriolevel6 aper.Enumerated = 5
	PagingPriorityPresentPriolevel7 aper.Enumerated = 6
	PagingPriorityPresentPriolevel8 aper.Enumerated = 7
)

type PagingPriority struct {
	Value aper.Enumerated `aper:"valueExt,valueLB:0,valueUB:7"`
}
