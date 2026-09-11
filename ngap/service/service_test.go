// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package service

import (
	"net"
	"testing"
	"time"

	"github.com/ishidawataru/sctp"
)

// listenerLifecycleUsed makes the once-per-process constraint enforced rather than
// merely documented: Run writes package state the connection goroutines read, and the
// accept loop it starts cannot be stopped, so a second lifecycle in one binary would
// race with the first. `go test -count=2` skips instead of misleading.
var listenerLifecycleUsed bool

// requireFreshListener also skips where the kernel has no SCTP support, which is the
// usual case in a container on a non-Linux host: without it, Listen fails inside Run's
// goroutine and the only symptom here is a dial timeout.
func requireFreshListener(t *testing.T) {
	t.Helper()

	if listenerLifecycleUsed {
		t.Skip("Run is once per process: the first lifecycle's accept loop cannot be stopped")
	}

	probe, err := sctp.ListenSCTP("sctp", &sctp.SCTPAddr{IPAddrs: []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}})
	if err != nil {
		t.Skipf("no SCTP support in this kernel: %v", err)
	}

	if err := probe.Close(); err != nil {
		t.Fatalf("closing the probe listener: %v", err)
	}

	listenerLifecycleUsed = true
}

// awaitListenerAddr reads the port the kernel chose. Run takes port 0 so that the test
// cannot collide with anything on the host, and the listener pointer is safe to read now
// that it is guarded.
func awaitListenerAddr(t *testing.T) *sctp.SCTPAddr {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if listener := currentListener(); listener != nil {
			addr, ok := listener.Addr().(*sctp.SCTPAddr)
			if !ok {
				t.Fatalf("listener address is %T, want *sctp.SCTPAddr", listener.Addr())
			}

			return addr
		}

		if time.Now().After(deadline) {
			t.Fatal("the listener did not bind within 5s")
		}

		time.Sleep(20 * time.Millisecond)
	}
}

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
	requireFreshListener(t)

	Run([]string{"127.0.0.1"}, 0, NGAPHandler{
		HandleMessage:      func(_ net.Conn, _ []byte) {},
		HandleNotification: func(_ net.Conn, _ []byte) {},
	})

	conn := dialUntilAccepting(t, awaitListenerAddr(t).Port)
	defer conn.Close()

	// A successful dial means the client finished its handshake, not that the server's
	// accept has returned and registered the association.
	awaitTrue(t, "the association to be registered", func() bool { return openAssociations() == 1 })

	Stop()

	// Reaching here at all is half of it: Stop used to panic before closing anything.
	// The other half is that the association was closed rather than walked past — its
	// handler unwinds and drops it from the map. Close shuts the socket down as well as
	// closing the descriptor, so the handler's blocked read returns promptly; measured at
	// about 14ms, against the 10s this allows.
	awaitTrue(t, "the association to be closed", func() bool { return openAssociations() == 0 })

	if listener := currentListener(); listener != nil {
		t.Errorf("Stop left the listener in place: %v", listener.Addr())
	}

	// Termination is entered once today, but a second Stop must find nothing to close:
	// sctp.SCTPListener.Close is a bare syscall.Close on a descriptor it does not
	// invalidate, so closing the same one again could close whatever descriptor number
	// the kernel had handed out in between.
	Stop()
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
