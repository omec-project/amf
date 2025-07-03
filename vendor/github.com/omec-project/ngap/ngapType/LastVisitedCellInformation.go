// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	LastVisitedCellInformationPresentNothing int = iota /* No components present */
	LastVisitedCellInformationPresentNGRANCell
	LastVisitedCellInformationPresentEUTRANCell
	LastVisitedCellInformationPresentUTRANCell
	LastVisitedCellInformationPresentGERANCell
	LastVisitedCellInformationPresentChoiceExtensions
)

type LastVisitedCellInformation struct {
	Present          int
	NGRANCell        *LastVisitedNGRANCellInformation `aper:"valueExt"`
	EUTRANCell       *LastVisitedEUTRANCellInformation
	UTRANCell        *LastVisitedUTRANCellInformation
	GERANCell        *LastVisitedGERANCellInformation
	ChoiceExtensions *ProtocolIESingleContainerLastVisitedCellInformationExtIEs
}
