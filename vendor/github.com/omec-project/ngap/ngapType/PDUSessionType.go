// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	PDUSessionTypePresentIpv4         aper.Enumerated = 0
	PDUSessionTypePresentIpv6         aper.Enumerated = 1
	PDUSessionTypePresentIpv4v6       aper.Enumerated = 2
	PDUSessionTypePresentEthernet     aper.Enumerated = 3
	PDUSessionTypePresentUnstructured aper.Enumerated = 4
)

type PDUSessionType struct {
	Value aper.Enumerated `aper:"valueExt,valueLB:0,valueUB:4"`
}
