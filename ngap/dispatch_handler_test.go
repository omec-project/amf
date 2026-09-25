// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package ngap

import (
	ctxt "context"
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/protos/sdcoreAmfServer"
	"github.com/omec-project/ngap/v2"
	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
)

// uplinkNasTransport encodes an Uplink NAS Transport naming ranUe, the message a gNB
// sends for a UE the AMF already has a context for.
func uplinkNasTransport(t *testing.T, ranUe *context.RanUe) []byte {
	t.Helper()

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
									NASPDU:  &ngapType.NASPDU{Value: []byte{0x7e, 0x00, 0x41}},
								},
							},
						},
					},
				},
			},
		},
	}
	raw, err := ngap.Encoder(pdu)
	if err != nil {
		t.Fatalf("encoding the Uplink NAS Transport: %v", err)
	}

	return raw
}

// An NGAP message for a UE the AMF has a context for is queued on the UE's event channel,
// and the message now carries the handler that runs it. As in the NAS test, a closure
// queued behind it runs only once the NGAP message has been handled; one queued without
// its handler would have ended the process first.
func TestAnNgapMessageForAKnownUeIsQueuedWithItsHandler(t *testing.T) {
	// FetchRanUeContext refuses all traffic until the AMF has its configuration.
	self := context.AMF_Self()
	originalRcvd := self.Rcvd
	self.Rcvd = true
	t.Cleanup(func() { self.Rcvd = originalRcvd })

	conn := &mockConn{}
	ran := context.AMF_Self().NewAmfRan(conn)
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	t.Cleanup(ran.Remove)

	ranUe, err := ran.NewRanUe(9702)
	if err != nil {
		t.Fatalf("NewRanUe: %v", err)
	}
	ranUe.Log = logger.NgapLog
	amfUe := context.AMF_Self().NewAmfUe("")
	amfUe.AttachRanUe(ranUe)
	t.Cleanup(func() {
		if amfUe.EventChannel != nil {
			amfUe.EventChannel.Event <- "quit"
		}
	})

	Dispatch(conn, uplinkNasTransport(t, ranUe))

	if amfUe.EventChannel == nil {
		t.Fatal("the NGAP message did not reach the UE's event channel")
		return
	}
	handled := make(chan struct{})
	amfUe.RunSerialized(func() { close(handled) })
	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatal("the queued NGAP message was never handled")
	}
}

// The SCTP load-balancer path queues the same message, from a RAN found by its gNB id.
func TestAnNgapMessageThroughTheLoadBalancerIsQueuedWithItsHandler(t *testing.T) {
	self := context.AMF_Self()
	originalRcvd := self.Rcvd
	self.Rcvd = true
	t.Cleanup(func() { self.Rcvd = originalRcvd })

	const gnbID = "208:93:0097ff"
	ran := self.NewAmfRanId(gnbID)
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	// Remove deletes the pool entry by connection outside SCTP LB mode, and a RAN made
	// from a gNB id has none -- so its entry, keyed by that id, goes explicitly.
	t.Cleanup(func() {
		ran.Remove()
		self.AmfRanPool.Delete(gnbID)
	})

	ranUe, err := ran.NewRanUe(9703)
	if err != nil {
		t.Fatalf("NewRanUe: %v", err)
	}
	ranUe.Log = logger.NgapLog
	amfUe := self.NewAmfUe("")
	amfUe.AttachRanUe(ranUe)
	t.Cleanup(func() {
		if amfUe.EventChannel != nil {
			amfUe.EventChannel.Event <- "quit"
		}
	})

	DispatchLb(ctxt.Background(), &sdcoreAmfServer.SctplbMessage{GnbId: gnbID, Msg: uplinkNasTransport(t, ranUe)},
		make(chan *sdcoreAmfServer.AmfMessage, 1))

	if amfUe.EventChannel == nil {
		t.Fatal("the NGAP message did not reach the UE's event channel")
		return
	}
	handled := make(chan struct{})
	amfUe.RunSerialized(func() { close(handled) })
	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatal("the queued NGAP message was never handled")
	}
}
