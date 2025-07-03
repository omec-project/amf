// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	CellIDListForRestartPresentNothing int = iota /* No components present */
	CellIDListForRestartPresentEUTRACGIListforRestart
	CellIDListForRestartPresentNRCGIListforRestart
	CellIDListForRestartPresentChoiceExtensions
)

type CellIDListForRestart struct {
	Present                int
	EUTRACGIListforRestart *EUTRACGIList
	NRCGIListforRestart    *NRCGIList
	ChoiceExtensions       *ProtocolIESingleContainerCellIDListForRestartExtIEs
}
