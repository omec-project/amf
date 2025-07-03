// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	NGAPPDUPresentNothing int = iota /* No components present */
	NGAPPDUPresentInitiatingMessage
	NGAPPDUPresentSuccessfulOutcome
	NGAPPDUPresentUnsuccessfulOutcome
	/* Extensions may appear below */
)

type NGAPPDU struct {
	Present             int
	InitiatingMessage   *InitiatingMessage
	SuccessfulOutcome   *SuccessfulOutcome
	UnsuccessfulOutcome *UnsuccessfulOutcome
}
