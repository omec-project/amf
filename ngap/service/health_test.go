// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net"
	"testing"
	"time"

	"github.com/ishidawataru/sctp"
	"github.com/omec-project/amf/context"
)

// withCleanState runs a case against a known listener state and puts back whatever the
// package held, so the cases below stay independent of their order.
func withCleanState(t *testing.T) {
	t.Helper()

	wasServed, wasShuttingDown, wasBindFailed := served.Load(), shuttingDown.Load(), bindFailed.Load()
	wasDirect := directSctpServing.Load()

	t.Cleanup(func() {
		served.Store(wasServed)
		shuttingDown.Store(wasShuttingDown)
		bindFailed.Store(wasBindFailed)
		directSctpServing.Store(wasDirect)
		connections.Range(func(key, _ any) bool {
			connections.Delete(key)
			return true
		})
	})

	served.Store(false)
	shuttingDown.Store(false)
	bindFailed.Store(false)
	connections.Range(func(key, _ any) bool {
		connections.Delete(key)
		return true
	})
}

// A fresh deployment has no associations and is not broken. Reporting it unhealthy would
// restart it in a loop before any gNB had the chance to connect, and this is also what
// keeps the signal quiet where the SCTP load balancer holds the associations instead.
func TestHealthyBeforeAnyAssociationHasBeenServed(t *testing.T) {
	withCleanState(t)

	if healthy, reason := Healthy(); !healthy {
		t.Errorf("Healthy() = false (%s) before any association, want true", reason)
	}
}

func TestHealthyWhileServing(t *testing.T) {
	withCleanState(t)

	conn, peer := net.Pipe()
	defer conn.Close()
	defer peer.Close()

	served.Store(true)
	connections.Store(conn, true)

	if healthy, reason := Healthy(); !healthy {
		t.Errorf("Healthy() = false (%s) while an association is held, want true", reason)
	}
}

// The state a process-level check cannot see: up, bound, answering, serving nobody.
func TestUnhealthyOnceEveryAssociationIsLost(t *testing.T) {
	withCleanState(t)

	served.Store(true)

	healthy, reason := Healthy()
	if healthy {
		t.Error("Healthy() = true after every association was lost, want false")
	}

	if reason == "" {
		t.Error("Healthy() gave no reason for being unhealthy, which is what a probe's logs need")
	}
}

// Losing every association is what a terminating AMF was asked to do, so it must not read
// as the fault above — otherwise every rollout restarts the pod it is already replacing.
func TestHealthyWhileShuttingDown(t *testing.T) {
	withCleanState(t)

	served.Store(true)
	shuttingDown.Store(true)

	if healthy, reason := Healthy(); !healthy {
		t.Errorf("Healthy() = false (%s) during shutdown, want true", reason)
	}
}

// withSctpLb runs a case against a known deployment mode and puts back what the context held.
func withSctpLb(t *testing.T, enabled bool) {
	t.Helper()

	self := context.AMF_Self()
	was := self.EnableSctpLb

	t.Cleanup(func() { self.EnableSctpLb = was })

	self.EnableSctpLb = enabled

	// Through the same capture Run performs, so these cases exercise the path production
	// takes rather than a field it no longer reads at check time.
	captureServingPath()
}

// listenAndServeFailingBind drives listenAndServe through a bind that cannot succeed, on any
// host. It takes a loopback port itself and asks listenAndServe for the same one, rather than
// relying on an address the machine does not own: with net.ipv4.ip_nonlocal_bind=1 such a bind
// succeeds, and listenAndServe then blocks in its accept loop for good.
//
// listenAndServe is run on its own goroutine so that a bind which does succeed fails the case
// instead of hanging it until the test binary's timeout.
func listenAndServeFailingBind(t *testing.T) {
	t.Helper()

	loopback := []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}

	holder, err := sctp.ListenSCTP("sctp", &sctp.SCTPAddr{IPAddrs: loopback})
	if err != nil {
		t.Skipf("cannot open an SCTP listener to occupy a port: %v", err)
	}

	t.Cleanup(func() { _ = holder.Close() })

	taken, ok := holder.Addr().(*sctp.SCTPAddr)
	if !ok || taken.Port == 0 {
		t.Fatalf("occupying listener reported %v, want an SCTP address with a port", holder.Addr())
	}

	done := make(chan struct{})

	go func() {
		defer close(done)

		listenAndServe(&sctp.SCTPAddr{IPAddrs: loopback, Port: taken.Port}, NGAPHandler{})
	}()

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatalf("listenAndServe did not return: it bound to a port that was already taken")
	}
}

// The fault above reached from the other side: an AMF whose listener never bound has served
// nobody and never can, and Run starts exactly one listenAndServe with nothing to retry it.
// Until this it answered the liveness endpoint with "no NGAP association has been served
// yet" - the same answer a healthy AMF waiting for its first gNB gives - so from outside the
// two were indistinguishable, and the element that could never serve was the one being left
// alone.
//
// This drives listenAndServe rather than setting the flag, because a test that stores
// bindFailed itself cannot fail if the bind path never stores it.
func TestUnhealthyWhenTheListenerNeverBound(t *testing.T) {
	withCleanState(t)
	withSctpLb(t, false)

	listenAndServeFailingBind(t)

	if !bindFailed.Load() {
		t.Fatal("listenAndServe returned without recording the bind failure")
	}

	healthy, reason := Healthy()
	if healthy {
		t.Errorf("Healthy() = true (%s) after the listener never bound, want false", reason)
	}

	if reason == "" {
		t.Error("Healthy() gave no reason for being unhealthy, which is what a probe's logs need")
	}
}

// Run is called whether or not the SCTP load balancer fronts this AMF, so an AMF that serves
// gNB traffic over gRPC from sctplb still opens this listener and then never uses it. A failed
// bind there says nothing about whether that AMF can serve, and restarting it for an unused
// socket would be the false positive this signal exists to avoid. What such a deployment's
// health does rest on - the gRPC listener - is not represented here at all, which is why this
// only declines to claim a fault rather than claiming health on its behalf.
func TestALoadBalancedAmfIsNotUnhealthyForAnUnusedListener(t *testing.T) {
	withCleanState(t)
	withSctpLb(t, true)

	listenAndServeFailingBind(t)

	if !bindFailed.Load() {
		t.Fatal("listenAndServe returned without recording the bind failure")
	}

	if healthy, reason := Healthy(); !healthy {
		t.Errorf("Healthy() = false (%s) for an AMF whose gNBs arrive through sctplb, want true", reason)
	}
}

// A pod that was told to terminate is going away whatever this says, so the shutdown
// exclusion has to sit above every fault - including one that outlives the pod, like a
// listener that never bound. Reporting a fault at that point cannot help anyone and
// contradicts what the exclusion is for.
func TestShuttingDownOutranksAFailedBind(t *testing.T) {
	withCleanState(t)
	withSctpLb(t, false)

	listenAndServeFailingBind(t)

	if !bindFailed.Load() {
		t.Fatal("listenAndServe returned without recording the bind failure")
	}

	shuttingDown.Store(true)

	if healthy, reason := Healthy(); !healthy {
		t.Errorf("Healthy() = false (%s) while terminating after a failed bind, want true", reason)
	}
}
