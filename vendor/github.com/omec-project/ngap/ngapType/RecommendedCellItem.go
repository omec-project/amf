// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type RecommendedCellItem struct {
	NGRANCGI         NGRANCGI                                             `aper:"valueLB:0,valueUB:2"`
	TimeStayedInCell *int64                                               `aper:"valueLB:0,valueUB:4095,optional"`
	IEExtensions     *ProtocolExtensionContainerRecommendedCellItemExtIEs `aper:"optional"`
}
