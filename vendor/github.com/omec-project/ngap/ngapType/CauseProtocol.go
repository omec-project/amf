// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
//
// SPDX-License-Identifier: Apache-2.0

package ngapType

import "github.com/omec-project/aper"

// Need to import "github.com/omec-project/aper" if it uses "aper"

const (
	CauseProtocolPresentTransferSyntaxError                          aper.Enumerated = 0
	CauseProtocolPresentAbstractSyntaxErrorReject                    aper.Enumerated = 1
	CauseProtocolPresentAbstractSyntaxErrorIgnoreAndNotify           aper.Enumerated = 2
	CauseProtocolPresentMessageNotCompatibleWithReceiverState        aper.Enumerated = 3
	CauseProtocolPresentSemanticError                                aper.Enumerated = 4
	CauseProtocolPresentAbstractSyntaxErrorFalselyConstructedMessage aper.Enumerated = 5
	CauseProtocolPresentUnspecified                                  aper.Enumerated = 6
)

type CauseProtocol struct {
	Value aper.Enumerated `aper:"valueExt,valueLB:0,valueUB:6"`
}
