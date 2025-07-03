// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	RRCEstablishmentCausePresentEmergency          aper.Enumerated = 0
	RRCEstablishmentCausePresentHighPriorityAccess aper.Enumerated = 1
	RRCEstablishmentCausePresentMtAccess           aper.Enumerated = 2
	RRCEstablishmentCausePresentMoSignalling       aper.Enumerated = 3
	RRCEstablishmentCausePresentMoData             aper.Enumerated = 4
	RRCEstablishmentCausePresentMoVoiceCall        aper.Enumerated = 5
	RRCEstablishmentCausePresentMoVideoCall        aper.Enumerated = 6
	RRCEstablishmentCausePresentMoSMS              aper.Enumerated = 7
	RRCEstablishmentCausePresentMpsPriorityAccess  aper.Enumerated = 8
	RRCEstablishmentCausePresentMcsPriorityAccess  aper.Enumerated = 9
	RRCEstablishmentCausePresentNotAvailable       aper.Enumerated = 10
)

type RRCEstablishmentCause struct {
	Value aper.Enumerated `aper:"valueExt,valueLB:0,valueUB:9"`
}
