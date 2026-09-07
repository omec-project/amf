// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net"
	"testing"
)

// withCleanState runs a case against a known listener state and puts back whatever the
// package held, so the four cases below stay independent of their order.
func withCleanState(t *testing.T) {
	t.Helper()

	wasServed, wasShuttingDown := served.Load(), shuttingDown.Load()

	t.Cleanup(func() {
		served.Store(wasServed)
		shuttingDown.Store(wasShuttingDown)
		connections.Range(func(key, _ any) bool {
			connections.Delete(key)
			return true
		})
	})

	served.Store(false)
	shuttingDown.Store(false)
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
