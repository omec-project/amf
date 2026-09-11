// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net"
	"testing"

	"github.com/ishidawataru/sctp"
	"github.com/omec-project/amf/context"
)

// withCleanState runs a case against a known listener state and puts back whatever the
// package held, so the cases below stay independent of their order.
func withCleanState(t *testing.T) {
	t.Helper()

	wasServed, wasShuttingDown, wasBindFailed := served.Load(), shuttingDown.Load(), bindFailed.Load()

	t.Cleanup(func() {
		served.Store(wasServed)
		shuttingDown.Store(wasShuttingDown)
		bindFailed.Store(wasBindFailed)
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

	// TEST-NET-1, which is not a local address, so the bind cannot succeed.
	listenAndServe(&sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}},
		Port:    38412,
	}, NGAPHandler{})

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

	listenAndServe(&sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("192.0.2.1")}},
		Port:    38412,
	}, NGAPHandler{})

	if !bindFailed.Load() {
		t.Fatal("listenAndServe returned without recording the bind failure")
	}

	if healthy, reason := Healthy(); !healthy {
		t.Errorf("Healthy() = false (%s) for an AMF whose gNBs arrive through sctplb, want true", reason)
	}
}
