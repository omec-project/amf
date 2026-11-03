// Copyright (C) 2026 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package ngap

import (
	"bytes"
	ctxt "context"
	"net"
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	libngap "github.com/omec-project/ngap/v2"
	"github.com/omec-project/ngap/v2/aper"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
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

// uplinkNasTransportWithLocation builds an Uplink NAS Transport for ranUe whose User Location Information
// carries a PLMN identity and a TAC, which the decoder keeps as sub-slices of its input.
func uplinkNasTransportWithLocation(t *testing.T, ranUe *context.RanUe, plmn, tac []byte) []byte {
	t.Helper()

	location := ngapType.UserLocationInformation{
		Present: ngapType.UserLocationInformationPresentUserLocationInformationNR,
		UserLocationInformationNR: &ngapType.UserLocationInformationNR{
			NRCGI: ngapType.NRCGI{
				PLMNIdentity:   ngapType.PLMNIdentity{Value: aper.OctetString(plmn)},
				NRCellIdentity: ngapType.NRCellIdentity{Value: aper.BitString{Bytes: []byte{0, 0, 0, 0, 0x10}, BitLength: 36}},
			},
			TAI: ngapType.TAI{
				PLMNIdentity: ngapType.PLMNIdentity{Value: aper.OctetString(plmn)},
				TAC:          ngapType.TAC{Value: aper.OctetString(tac)},
			},
		},
	}
	pdu := ngapType.NGAPPDU{
		Present: ngapType.NGAPPDUPresentInitiatingMessage,
		InitiatingMessage: &ngapType.InitiatingMessage{
			ProcedureCode: ngapType.ProcedureCode{Value: ngapType.ProcedureCodeUplinkNASTransport},
			Criticality:   ngapType.Criticality{Value: ngapType.CriticalityPresentIgnore},
			Value: ngapType.InitiatingMessageValue{
				Present: ngapType.InitiatingMessagePresentUplinkNASTransport,
				UplinkNASTransport: &ngapType.UplinkNASTransport{
					ProtocolIEs: ngapType.ProtocolIEContainerUplinkNASTransportIEs{
						List: []ngapType.UplinkNASTransportIEs{
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDAMFUENGAPID},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.UplinkNASTransportIEsValue{
									Present:     ngapType.UplinkNASTransportIEsPresentAMFUENGAPID,
									AMFUENGAPID: &ngapType.AMFUENGAPID{Value: ranUe.AmfUeNgapId},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDRANUENGAPID},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.UplinkNASTransportIEsValue{
									Present:     ngapType.UplinkNASTransportIEsPresentRANUENGAPID,
									RANUENGAPID: &ngapType.RANUENGAPID{Value: ranUe.RanUeNgapId},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDNASPDU},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
								Value: ngapType.UplinkNASTransportIEsValue{
									Present: ngapType.UplinkNASTransportIEsPresentNASPDU,
									NASPDU:  &ngapType.NASPDU{Value: aper.OctetString{0x7e, 0x00}},
								},
							},
							{
								Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDUserLocationInformation},
								Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentIgnore},
								Value: ngapType.UplinkNASTransportIEsValue{
									Present:                 ngapType.UplinkNASTransportIEsPresentUserLocationInformation,
									UserLocationInformation: &location,
								},
							},
						},
					},
				},
			},
		},
	}
	encoded, err := libngap.Encoder(pdu)
	if err != nil {
		t.Fatalf("encoding the Uplink NAS Transport: %v", err)
	}
	return encoded
}

// The caller of Dispatch reuses its read buffer, so a message queued for a UE must not alias it: the
// buffer is overwritten after Dispatch returns, while the message is still waiting behind another
// one on the UE's event channel, and the decoded PLMN identity and TAC must keep their values.
//
// This guards the property, not the copy in Dispatch: with the aper version in go.mod the decoder
// copies the contents of every IE (open type) itself, so the test also passes without that copy.
func TestDispatchQueuedMessageDoesNotAliasTheReadBuffer(t *testing.T) {
	self := context.AMF_Self()
	previousReady := self.Rcvd
	self.Rcvd = true
	t.Cleanup(func() { self.Rcvd = previousReady })

	conn := &mockConn{}
	ran := self.NewAmfRan(conn)
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	t.Cleanup(func() { self.AmfRanPool.Delete(conn) })

	const supi = "imsi-208930100009995"
	amfUe := self.NewAmfUe(supi)
	ranUe, err := ran.NewRanUe(995)
	if err != nil {
		t.Fatalf("creating RanUe: %v", err)
	}
	amfUe.AttachRanUe(ranUe)
	t.Cleanup(func() {
		if leftover := self.RanUeFindByAmfUeNgapIDLocal(ranUe.AmfUeNgapId); leftover != nil {
			if err := leftover.Remove(); err != nil {
				t.Logf("removing the leftover RanUe: %v", err)
			}
		}
		self.UePool.Delete(supi)
	})

	// Hold the UE's event channel on a message of its own, so what Dispatch queues stays queued.
	started, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release) })
	amfUe.RunSerialized(func() {
		close(started)
		<-release
	})
	<-started

	wantPlmn, wantTac := []byte{0x02, 0xf8, 0x39}, []byte{0x00, 0x00, 0x01}
	buf := uplinkNasTransportWithLocation(t, ranUe, wantPlmn, wantTac)
	Dispatch(conn, buf)

	for i := range buf {
		buf[i] = 0xff
	}

	var queued context.NgapMsg
	select {
	case msg := <-amfUe.EventChannel.Message:
		queued = msg.(context.NgapMsg)
	default:
		t.Fatal("Dispatch did not queue the message on the UE's event channel")
	}

	var location *ngapType.UserLocationInformation
	for _, ie := range queued.NgapMsg.InitiatingMessage.Value.UplinkNASTransport.ProtocolIEs.List {
		if ie.Id.Value == ngapType.ProtocolIEIDUserLocationInformation {
			location = ie.Value.UserLocationInformation
		}
	}
	if location == nil || location.UserLocationInformationNR == nil {
		t.Fatal("the queued message has no NR User Location Information")
	}
	tai := location.UserLocationInformationNR.TAI
	if !bytes.Equal(tai.PLMNIdentity.Value, wantPlmn) {
		t.Errorf("PLMN identity = % x, want % x: the decoded message aliases the read buffer", tai.PLMNIdentity.Value, wantPlmn)
	}
	if !bytes.Equal(tai.TAC.Value, wantTac) {
		t.Errorf("TAC = % x, want % x: the decoded message aliases the read buffer", tai.TAC.Value, wantTac)
	}
}
