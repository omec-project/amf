// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

type LocationReportingRequestType struct {
	EventType                                 EventType
	ReportArea                                ReportArea
	AreaOfInterestList                        *AreaOfInterestList                                           `aper:"optional"`
	LocationReportingReferenceIDToBeCancelled *LocationReportingReferenceID                                 `aper:"optional"`
	IEExtensions                              *ProtocolExtensionContainerLocationReportingRequestTypeExtIEs `aper:"optional"`
}
