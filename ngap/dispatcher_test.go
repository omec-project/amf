// Copyright 2026 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package ngap

import (
	ctxt "context"
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	"go.uber.org/zap"
)

func TestDispatchLbIgnoresMissingRanContext(t *testing.T) {
	msg := &sdcoreAmfServer.SctplbMessage{
		Msg: []byte{0x00},
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("DispatchLb panicked with missing RAN identity: %v", recovered)
		}
	}()

	DispatchLb(ctxt.Background(), msg, make(chan *sdcoreAmfServer.AmfMessage, 1))
}

func TestDispatchNgapMsgIgnoresNilPdu(t *testing.T) {
	ran := &context.AmfRan{Log: zap.NewNop().Sugar()}

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("DispatchNgapMsg panicked with nil PDU: %v", recovered)
		}
	}()

	DispatchNgapMsg(ctxt.Background(), ran, nil, nil)
}
