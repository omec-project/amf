// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/util/li/x1"
	"github.com/omec-project/util/li/x2x3"
)

// loopbackPKI is liPKI with the key pair as well, so a stub mediation function on the
// loopback can present the same certificate — which carries an IP SAN — and be *verified*
// rather than merely trusted, so the sender's handshake with it completes.
func loopbackPKI(t *testing.T) (certPath, keyPath, caPath string, pair tls.Certificate) {
	t.Helper()

	certPath, keyPath, caPath = liPKI(t)

	return certPath, keyPath, caPath, liTrust(t).pair
}

// nonReadingMDF2 completes the TLS handshake on every connection it accepts and then never
// reads. handshaken is closed when the first handshake completes.
func nonReadingMDF2(t *testing.T, pair tls.Certificate) (addr string, handshaken <-chan struct{}) {
	t.Helper()

	ln, err := tls.Listen("tcp", testX1Listen, &tls.Config{
		Certificates: []tls.Certificate{pair},
		ClientAuth:   tls.RequireAnyClientCert,
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() }) //nolint:errcheck // test cleanup

	held := make(chan struct{})
	t.Cleanup(func() { close(held) })

	done := make(chan struct{})

	var once sync.Once

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close() //nolint:errcheck // test
				if err := conn.(*tls.Conn).HandshakeContext(context.Background()); err == nil {
					once.Do(func() { close(done) })
				}
				<-held
			}()
		}
	}()

	return ln.Addr().String(), done
}

// TestProductDroppedByAFullQueueIsReported pins that a PDU the delivery queue drops for want of
// space reaches the ADMF as x2DeliveryLost. It is driven through Init rather than through a pool
// a test builds itself, because what it guards is the wiring: the drop hook Init hands the pool.
// The worker's unreachability report does not cover these drops. AsyncSender.Unreachable
// excludes queue saturation deliberately — a full queue at one instant is a burst the buffer
// exists to absorb — and says in the same paragraph that the drops themselves are reported as
// they happen, so without the hook a dropped xIRI is reported nowhere.
//
// The queue overflows because the burst is offered faster than the worker drains it, with the
// sender connected to a mediation function that has completed the handshake. This does not
// show that a slow peer causes the overflow: the loopback socket buffers absorb what the
// worker writes.
func TestProductDroppedByAFullQueueIsReported(t *testing.T) {
	cert, key, ca, pair := loopbackPKI(t)
	admf := newADMFStub(t)
	mdf2, handshaken := nonReadingMDF2(t, pair)
	t.Cleanup(func() { active.Store(nil) })

	if err := Init(Config{
		NEID:     testNEID,
		X1Listen: testX1Listen,
		MDF2:     mdf2,
		Cert:     cert, Key: key, CACert: ca,
		AdmfURL: admf.srv.URL, AdmfID: testADMFID,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}

	sub := active.Load()
	if sub == nil {
		t.Fatal("interception did not start")
	}
	s := sub.senderFor(mdf2)

	// The PDU has the shape production builds, so it frames and the worker writes it to the
	// connection rather than failing it before the write.
	pdu := &x2x3.PDU{
		Type:          x2x3.PDUTypeX2,
		PayloadFormat: x2x3.PayloadFormat3GPP33128,
		Direction:     x2x3.DirectionNotApplicable,
		Payload:       make([]byte, 512),
	}
	if _, err := pdu.Marshal(); err != nil {
		t.Fatalf("the test PDU does not frame: %v", err)
	}

	// One PDU first, so the worker dials and the handshake completes before the burst: the
	// drops below are then made with the destination connected, not while the worker is still
	// dialling.
	//nolint:errcheck // enqueueing cannot fail in a way this test acts on
	_ = s.Send(pdu)
	select {
	case <-handshaken:
	case <-time.After(10 * time.Second):
		t.Fatal("the sender never completed a TLS handshake with the stub mediation function")
	}

	// Comfortably past the queue's 1024, offered faster than the worker drains them.
	for range 8000 {
		//nolint:errcheck // enqueueing cannot fail in a way this test acts on
		_ = s.Send(pdu)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(strings.Join(admf.received(), "\n"), "x2DeliveryLost") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("product was dropped for want of queue space and nothing was reported:\n%s",
		strings.Join(admf.received(), "\n"))
}

// TestADroppedUnitIsReportedAsALossAndNotAsUnreachability is the consumer half, asserted
// where the decision is made.
//
// A partial write costs one product unit only where the library's own resend of that unit
// does not land either — it resends it whole on the fresh connection, which recovers the
// ordinary case. What is left is a genuine loss to a destination that took the rest of the
// batch, and the library reports it as ErrUnitDropped precisely so it is not mistaken for
// unreachability: a healthy mediation function reported unreachable would have the watcher
// raise a fault about a working peer and retract it on the next send.
//
// Were this element to discard the error and only nudge the watcher, the watcher would sample
// a destination it correctly considers reachable, and the loss the library declines to report
// as unreachability would be reported nowhere at all — product missing from an agency's record
// with every channel that could have said so reporting normality.
//
// The transport that produces the error is tested where it lives (li/x2x3's partial-write
// suite, which drives a real socket). What this element does with it is this hook, and
// driving a real partial write to reach it would be asserting the other half twice while
// leaving this one's outcome to the machine's socket buffers.
func TestADroppedUnitIsReportedAsALossAndNotAsUnreachability(t *testing.T) {
	cert, key, ca, _ := loopbackPKI(t)
	admf := newADMFStub(t)
	t.Cleanup(func() { active.Store(nil) })

	if err := Init(Config{
		NEID:     testNEID,
		X1Listen: testX1Listen,
		MDF2:     testAgencyAddr,
		Cert:     cert, Key: key, CACert: ca,
		AdmfURL: admf.srv.URL, AdmfID: testADMFID,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	sub := active.Load()
	if sub == nil {
		t.Fatal("interception did not start")
	}

	// A watcher over no destinations, so the nudge has somewhere to go and reports nothing
	// of its own: what is asserted here is what the hook says, not what the watcher samples.
	watcher := x1.NewDestinationWatcher(func() []x1.DestinationHealth { return nil }, sub.reporter, 0)

	// The error as the library returns it: wrapped, because a caller must match it with
	// errors.Is rather than by comparison.
	sub.reportDeliveryError(fmt.Errorf("%w: send to 10.0.60.122:42069", x2x3.ErrUnitDropped), watcher)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(strings.Join(admf.received(), "\n"), "x2DeliveryLost") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("an xIRI was dropped on the way to a reachable mediation function and reported by "+
		"nothing:\n%s", strings.Join(admf.received(), "\n"))
}

// And the other direction, which keeps every delivery failure from becoming a product-loss
// report: an ordinary transport failure says the destination is not
// working, which is the watcher's to report at the scope that names it, not a loss of
// product this element can name.
func TestAnOrdinaryDeliveryFailureIsNotReportedAsALoss(t *testing.T) {
	cert, key, ca, _ := loopbackPKI(t)
	admf := newADMFStub(t)
	t.Cleanup(func() { active.Store(nil) })

	if err := Init(Config{
		NEID:     testNEID,
		X1Listen: testX1Listen,
		MDF2:     testAgencyAddr,
		Cert:     cert, Key: key, CACert: ca,
		AdmfURL: admf.srv.URL, AdmfID: testADMFID,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	sub := active.Load()
	if sub == nil {
		t.Fatal("interception did not start")
	}
	watcher := x1.NewDestinationWatcher(func() []x1.DestinationHealth { return nil }, sub.reporter, 0)

	sub.reportDeliveryError(errors.New("dial tcp 10.0.60.122:42069: connection refused"), watcher)

	// Nothing to wait for, so give the asynchronous path a moment to be wrong in.
	time.Sleep(200 * time.Millisecond)
	if joined := strings.Join(admf.received(), "\n"); strings.Contains(joined, "x2DeliveryLost") {
		t.Errorf("a connection failure was reported as lost product:\n%s", joined)
	}
}

// TestADroppedRunOfXIRIsIsReportedWithItsCount: a resend that does not land loses every unit
// from the one the write stopped in to the end of its batch, and the library counts them. The
// report must say how many, or the agency's gap is larger than this element's account of it.
func TestADroppedRunOfXIRIsIsReportedWithItsCount(t *testing.T) {
	cert, key, ca, _ := loopbackPKI(t)
	admf := newADMFStub(t)
	t.Cleanup(func() { active.Store(nil) })

	if err := Init(Config{
		NEID:     testNEID,
		X1Listen: testX1Listen,
		MDF2:     testAgencyAddr,
		Cert:     cert, Key: key, CACert: ca,
		AdmfURL: admf.srv.URL, AdmfID: testADMFID,
	}); err != nil {
		t.Fatalf("Init: %v", err)
	}
	sub := active.Load()
	if sub == nil {
		t.Fatal("interception did not start")
	}
	watcher := x1.NewDestinationWatcher(func() []x1.DestinationHealth { return nil }, sub.reporter, 0)

	sub.reportDeliveryError(&x2x3.DeliveryError{
		Lost: 5,
		Err:  fmt.Errorf("%w: send to 10.0.60.122:42069", x2x3.ErrUnitDropped),
	}, watcher)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		joined := strings.Join(admf.received(), "\n")
		if strings.Contains(joined, "x2DeliveryLost") {
			if !strings.Contains(joined, "5 xIRIs") {
				t.Errorf("the loss was reported without the number of xIRIs it cost:\n%s", joined)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("a dropped run of xIRIs was reported by nothing:\n%s", strings.Join(admf.received(), "\n"))
}
