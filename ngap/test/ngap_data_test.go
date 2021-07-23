// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
// SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

package ngap_test

import (
	"net"
	"time"
)

// ASN.1 Basic-PER encoded values
const (
	ngapPDUIncomingMessage     byte = 0x00
	ngapPDUSuccessfulOutcome   byte = 0x20
	ngapPDUUnSuccessfulOutcome byte = 0x40
)

var messageTypeMap = map[byte]string{
	ngapPDUIncomingMessage:     "IncomingMessage",
	ngapPDUSuccessfulOutcome:   "SuccessfulOutcome",
	ngapPDUUnSuccessfulOutcome: "UnsuccessfulOutcome",
}

// Note: The dummy NGAP-PDUs are in accordance with the AMF configuration file
// amf/amfTest/amfcfg.yaml. Modifying amfcfg.yaml may affect the unit tests.

// NGAP-PDU for NGSetupReq message with TA supported by AMF
var testNGSetupReq = []byte{
	0x00, 0x15, 0x00, 0x37, 0x00, 0x00, 0x04, 0x00,
	0x1b, 0x00, 0x08, 0x00, 0x02, 0xf8, 0x39, 0x00,
	0x00, 0x00, 0x08, 0x00, 0x52, 0x40, 0x06, 0x01,
	0x80, 0x47, 0x4e, 0x42, 0x32, 0x00, 0x66, 0x00,
	0x15, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x02,
	0xf8, 0x39, 0x00, 0x01, 0x10, 0x18, 0x03, 0x06,
	0x09, 0x10, 0x08, 0x11, 0x22, 0x33, 0x00, 0x15,
	0x40, 0x01, 0x00,
}

// NGAP-PDU for NGSetupReq message with TA not supported by AMF
var testNGSetupReqErr = []byte{
	0x00, 0x15, 0x00, 0x37, 0x00, 0x00, 0x04, 0x00,
	0x1b, 0x00, 0x08, 0x00, 0x02, 0xf8, 0x39, 0x00,
	0x00, 0x00, 0x08, 0x00, 0x52, 0x40, 0x06, 0x01,
	0x80, 0x47, 0x4e, 0x42, 0x32, 0x00, 0x66, 0x00,
	0x15, 0x00, 0x00, 0x00, 0x00, 0x04, 0x00, 0x02,
	0xf8, 0x39, 0x00, 0x01, 0x10, 0x18, 0x03, 0x06,
	0x09, 0x10, 0x08, 0x11, 0x22, 0x33, 0x00, 0x15,
	0x40, 0x01, 0x00,
}

// Mock Connection struct. Implements the net.Conn interface
type testConn struct {
	data []byte
}

type testConnAddr struct {
}

func (tca testConnAddr) Network() (a string) { return }
func (tca testConnAddr) String() (a string)  { return }

// Write method of the mocked testConn struct will be invoked as a part of the
// unit test framework
func (tc *testConn) Write(b []byte) (n int, err error) {
	tc.data = b
	return
}

func (tc *testConn) Close() (e error) { return }

func (tc *testConn) Read(b []byte) (n int, err error) { return }

func (tc *testConn) LocalAddr() net.Addr                    { return testConnAddr{} }
func (tc *testConn) RemoteAddr() net.Addr                   { return testConnAddr{} }
func (tc *testConn) SetDeadline(t time.Time) (e error)      { return }
func (tc *testConn) SetReadDeadline(t time.Time) (e error)  { return }
func (tc *testConn) SetWriteDeadline(t time.Time) (e error) { return }
