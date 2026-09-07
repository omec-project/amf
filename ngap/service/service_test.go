// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net"
	"testing"
	"time"

	"github.com/ishidawataru/sctp"
)

const testPort = 39412

// dialUntilAccepting waits for the listener Run started on its own goroutine to be
// accepting, by dialling it rather than by reading the package's listener pointer.
func dialUntilAccepting(t *testing.T, port int) *sctp.SCTPConn {
	t.Helper()

	addr := &sctp.SCTPAddr{
		IPAddrs: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}},
		Port:    port,
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := sctp.DialSCTP("sctp", nil, addr)
		if err == nil {
			return conn
		}

		if time.Now().After(deadline) {
			t.Fatalf("no SCTP association accepted on port %d within 5s: %v", port, err)
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// openAssociations counts what the listener currently holds.
func openAssociations() int {
	open := 0
	connections.Range(func(_, _ any) bool {
		open++
		return true
	})

	return open
}

func awaitTrue(t *testing.T, what string, condition func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Errorf("timed out after 10s waiting for %s", what)
			return
		}

		time.Sleep(20 * time.Millisecond)
	}
}

// Stop is the only path that closes the AMF's NGAP associations, and it runs from
// service/init.go during termination — before the AMF tells its peers that it is going
// away. Nothing exercised it, so nothing noticed that the map it walks holds the
// association as its key and a bool as its value: with a single association open, Stop
// panicked on the type assertion and took the rest of the termination sequence with it.
//
// One listener lifecycle per process, because Run writes package state that the
// connection goroutines read; a second Run here would race with handlers still unwinding
// from the first.
func TestStopTearsDownTheListener(t *testing.T) {
	Run([]string{"127.0.0.1"}, testPort, NGAPHandler{
		HandleMessage:      func(_ net.Conn, _ []byte) {},
		HandleNotification: func(_ net.Conn, _ []byte) {},
	})

	conn := dialUntilAccepting(t, testPort)
	defer conn.Close()

	// A successful dial means the client finished its handshake, not that the server's
	// accept has returned and registered the association.
	awaitTrue(t, "the association to be registered", func() bool { return openAssociations() == 1 })

	Stop()

	// Reaching here at all is half of it: Stop used to panic before closing anything.
	// The other half is that the association was closed rather than walked past — its
	// handler unwinds and drops it from the map, which is observable without depending
	// on when the peer's own read wakes.
	awaitTrue(t, "the association to be closed", func() bool { return openAssociations() == 0 })
}

// Termination can beat the bind, and Listen can fail outright, in which case there is no
// listener to close. Stop still has to return: the notifications that tell this AMF's
// peers it is unavailable are sequenced after it.
func TestStopWithoutAListenerReturns(t *testing.T) {
	previous := currentListener()
	t.Cleanup(func() { setListener(previous) })

	setListener(nil)

	Stop()
}
