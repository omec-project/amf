// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	BroadcastCancelledAreaListPresentNothing int = iota /* No components present */
	BroadcastCancelledAreaListPresentCellIDCancelledEUTRA
	BroadcastCancelledAreaListPresentTAICancelledEUTRA
	BroadcastCancelledAreaListPresentEmergencyAreaIDCancelledEUTRA
	BroadcastCancelledAreaListPresentCellIDCancelledNR
	BroadcastCancelledAreaListPresentTAICancelledNR
	BroadcastCancelledAreaListPresentEmergencyAreaIDCancelledNR
	BroadcastCancelledAreaListPresentChoiceExtensions
)

type BroadcastCancelledAreaList struct {
	Present                       int
	CellIDCancelledEUTRA          *CellIDCancelledEUTRA
	TAICancelledEUTRA             *TAICancelledEUTRA
	EmergencyAreaIDCancelledEUTRA *EmergencyAreaIDCancelledEUTRA
	CellIDCancelledNR             *CellIDCancelledNR
	TAICancelledNR                *TAICancelledNR
	EmergencyAreaIDCancelledNR    *EmergencyAreaIDCancelledNR
	ChoiceExtensions              *ProtocolIESingleContainerBroadcastCancelledAreaListExtIEs
}
