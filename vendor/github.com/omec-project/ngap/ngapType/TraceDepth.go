// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	TraceDepthPresentMinimum                               aper.Enumerated = 0
	TraceDepthPresentMedium                                aper.Enumerated = 1
	TraceDepthPresentMaximum                               aper.Enumerated = 2
	TraceDepthPresentMinimumWithoutVendorSpecificExtension aper.Enumerated = 3
	TraceDepthPresentMediumWithoutVendorSpecificExtension  aper.Enumerated = 4
	TraceDepthPresentMaximumWithoutVendorSpecificExtension aper.Enumerated = 5
)

type TraceDepth struct {
	Value aper.Enumerated `aper:"valueExt,valueLB:0,valueUB:5"`
}
