// Copyright (C) 2026 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package ngap

import (
	ctxt "context"
	"net"
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
)

// mockConn is a minimal net.Conn used to exercise Dispatch without a real SCTP socket.
type mockConn struct{}

type mockAddr struct{}

func (a mockAddr) Network() string { return "sctp" }
func (a mockAddr) String() string  { return "192.0.2.1:38412" }

func (c *mockConn) RemoteAddr() net.Addr               { return mockAddr{} }
func (c *mockConn) LocalAddr() net.Addr                { return mockAddr{} }
func (c *mockConn) Read(_ []byte) (int, error)         { return 0, net.ErrClosed }
func (c *mockConn) Write(_ []byte) (int, error)        { return 0, net.ErrClosed }
func (c *mockConn) Close() error                       { return nil }
func (c *mockConn) SetDeadline(_ time.Time) error      { return nil }
func (c *mockConn) SetReadDeadline(_ time.Time) error  { return nil }
func (c *mockConn) SetWriteDeadline(_ time.Time) error { return nil }

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
	ran := context.NewAmfRanDefault()

	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("DispatchNgapMsg panicked with nil PDU: %v", recovered)
		}
	}()

	DispatchNgapMsg(ctxt.Background(), ran, nil, nil)
}

// TestDispatchEmptyMsgUnknownConnDoesNotPanic verifies that Dispatch does not panic
// and does not create an AmfRan entry when it receives an empty (connection-close)
// message for a connection that has no existing RAN context.
func TestDispatchEmptyMsgUnknownConnDoesNotPanic(t *testing.T) {
	conn := &mockConn{}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Dispatch panicked with empty message and unknown conn: %v", r)
		}
	}()

	Dispatch(conn, nil)

	// Confirm that no AmfRan was created in the pool for this unknown connection.
	if _, ok := context.AMF_Self().AmfRanFindByConn(conn); ok {
		t.Error("Dispatch must not create an AmfRan for an unknown connection with an empty message")
	}
}
