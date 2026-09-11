// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package service

import (
	"encoding/hex"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/ishidawataru/sctp"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/ngap/v2"
)

type NGAPHandler struct {
	HandleMessage      func(conn net.Conn, msg []byte)
	HandleNotification func(conn net.Conn, notificationData []byte)
}

const readBufSize uint32 = 131072

// set default read timeout to 2 seconds
var readTimeout syscall.Timeval = syscall.Timeval{Sec: 2, Usec: 0}

var (
	sctpListener *sctp.SCTPListener
	connections  sync.Map
	countMu      sync.Mutex

	// listenerMu guards sctpListener, which listenAndServe writes on the goroutine Run
	// starts and Stop reads on the goroutine that is terminating the AMF.
	listenerMu sync.RWMutex

	// served latches when this AMF first accepts an association. It is what makes
	// "holds none" a different state from "has never held one", and the reason a freshly
	// deployed AMF that no gNB has reached yet is not reported unhealthy.
	served atomic.Bool
	// shuttingDown records that Stop was called, so that an orderly termination closing
	// its associations does not read as the fault this detects.
	shuttingDown atomic.Bool
	// bindFailed records that the listener could not be created at all. Run starts exactly
	// one listenAndServe and nothing retries it, so the failure is terminal: this AMF will
	// never serve anyone. Without it such an AMF answers the liveness endpoint exactly as a
	// healthy one still waiting for its first gNB does.
	bindFailed atomic.Bool
)

func setListener(listener *sctp.SCTPListener) {
	listenerMu.Lock()
	sctpListener = listener
	listenerMu.Unlock()
}

func currentListener() *sctp.SCTPListener {
	listenerMu.RLock()
	defer listenerMu.RUnlock()

	return sctpListener
}

// takeListener returns the listener and clears it in the same acquisition, so a second Stop
// has nothing left to close. sctp.SCTPListener.Close is a bare syscall.Close on a descriptor
// it does not invalidate, unlike a net.Listener, so closing the same one twice would close
// whatever descriptor number the kernel had handed out in between.
func takeListener() *sctp.SCTPListener {
	listenerMu.Lock()
	defer listenerMu.Unlock()

	listener := sctpListener
	sctpListener = nil

	return listener
}

var handler NGAPHandler

var sctpConfig sctp.SocketConfig = sctp.SocketConfig{
	InitMsg: sctp.InitMsg{
		NumOstreams:    3,
		MaxInstreams:   5,
		MaxAttempts:    2,
		MaxInitTimeout: 2,
	},
	NotificationHandler: func(notificationData []byte) error {
		logger.NgapLog.Debugf("received SCTP notification of size %d bytes", len(notificationData))

		if handler.HandleNotification != nil {
			handler.HandleNotification(nil, notificationData)
		}
		return nil
	},
}

func Run(addresses []string, port int, h NGAPHandler) {
	handler = h

	ips := []net.IPAddr{}

	for _, addr := range addresses {
		if netAddr, err := net.ResolveIPAddr("ip", addr); err != nil {
			logger.NgapLog.Errorf("error resolving address '%s': %v\n", addr, err)
		} else {
			logger.NgapLog.Debugf("resolved address '%s' to %s\n", addr, netAddr)
			ips = append(ips, *netAddr)
		}
	}

	addr := &sctp.SCTPAddr{
		IPAddrs: ips,
		Port:    port,
	}

	go listenAndServe(addr, h)
}

func listenAndServe(addr *sctp.SCTPAddr, handler NGAPHandler) {
	listener, err := sctpConfig.Listen("sctp", addr)
	if err != nil {
		logger.NgapLog.Errorf("failed to listen: %+v", err)
		bindFailed.Store(true)

		return
	}

	setListener(listener)

	logger.NgapLog.Infof("Listen on %s", listener.Addr())

	for {
		newConn, err := listener.AcceptSCTP()
		if err != nil {
			switch err {
			case syscall.EINTR, syscall.EAGAIN:
				logger.NgapLog.Debugf("AcceptSCTP: %+v", err)
			default:
				logger.NgapLog.Errorf("failed to accept: %+v", err)
			}
			continue
		}

		var info *sctp.SndRcvInfo
		if infoTmp, errGet := newConn.GetDefaultSentParam(); errGet != nil {
			logger.NgapLog.Errorf("get default sent param error: %+v, accept failed", errGet)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			info = infoTmp
			logger.NgapLog.Debugf("get default sent param[value: %+v]", info)
		}

		info.PPID = ngap.PPID
		if errSet := newConn.SetDefaultSentParam(info); errSet != nil {
			logger.NgapLog.Errorf("set default sent param error: %+v, accept failed", errSet)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugf("set default sent param[value: %+v]", info)
		}

		events := sctp.SCTP_EVENT_DATA_IO | sctp.SCTP_EVENT_SHUTDOWN | sctp.SCTP_EVENT_ASSOCIATION
		if errSubs := newConn.SubscribeEvents(events); errSubs != nil {
			logger.NgapLog.Errorf("failed to accept: %+v", errSubs)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugln("subscribe SCTP event[DATA_IO, SHUTDOWN_EVENT, ASSOCIATION_CHANGE]")
		}

		if errSetR := newConn.SetReadBuffer(int(readBufSize)); errSetR != nil {
			logger.NgapLog.Errorf("set read buffer error: %+v, accept failed", errSetR)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugf("Set read buffer to %d bytes", readBufSize)
		}

		// Set read timeout using SO_RCVTIMEO socket option
		// This is the proper way to set timeouts on SCTP sockets
		rawConn, err := newConn.SyscallConn()
		if err != nil {
			logger.NgapLog.Errorf("get syscall conn error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		}

		var setTimeoutErr error
		err = rawConn.Control(func(fd uintptr) {
			setTimeoutErr = syscall.SetsockoptTimeval(int(fd), syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &readTimeout)
		})
		if err != nil || setTimeoutErr != nil {
			logger.NgapLog.Errorf("set read timeout error: control=%+v, setsockopt=%+v, accept failed", err, setTimeoutErr)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugf("set read timeout: %+v", readTimeout)
		}

		logger.NgapLog.Infof("[AMF] SCTP Accept from: %+v", newConn.RemoteAddr())
		connections.Store(newConn, true)
		served.Store(true)
		reportAssociationCount()

		go handleConnection(newConn, readBufSize, handler)
	}
}

// reportAssociationCount publishes how many associations the listener holds. The map is
// the authority for that number: AmfRanPool is keyed three ways — by connection, by remote
// address and by GnbId — so its length counts keys rather than associations.
//
// The count and the publish are taken under one lock so that two associations changing at
// once cannot leave the gauge holding the earlier of the two counts until the next event.
func reportAssociationCount() {
	countMu.Lock()
	defer countMu.Unlock()

	metrics.SetNgapAssociations(associationCount())
}

// associationCount reports how many SCTP associations this AMF currently terminates.
func associationCount() int {
	count := 0
	connections.Range(func(_, _ any) bool {
		count++
		return true
	})

	return count
}

// Healthy reports whether this AMF can serve a radio access network, and why not when it
// cannot.
//
// It is false in exactly one case: the AMF has served at least one association and now
// holds none, with no shutdown requested. That is the state a process-level check cannot
// see — the AMF is up, its listener is bound, its SBI answers, and nobody is being
// served, which looks identical to an idle deployment.
//
// A restart recovers the element. Whether it recovers service depends on the radio access
// network re-establishing its association, which not every implementation does, so the
// value claimed here is that the condition ends visibly rather than that traffic resumes.
//
// It also cannot loop: the latch lives in the process, so a restarted AMF has served
// nothing and reports healthy until a gNB attaches. A RAN that never comes back therefore
// costs exactly one restart, not a restart every failureThreshold.
//
// The two exclusions matter as much as the rule. An AMF that has never served is healthy,
// or a fresh deployment would restart in a loop before any gNB had the chance to connect,
// and no initial delay can cover a rig that sits deployed for hours first. An AMF that is
// terminating is healthy, because losing its associations is what it was asked to do.
//
// The latch also keeps this quiet where it does not apply: in a deployment whose gNBs
// connect to the SCTP load balancer, this AMF accepts no associations of its own, never
// latches, and so is never reported unhealthy by it.
func Healthy() (bool, string) {
	switch {
	// First, because nothing that follows can make it untrue: an AMF that never bound has
	// served nobody and never will, which is the same fault as one that has stopped
	// serving, reached from the other side.
	case bindFailed.Load():
		return false, "the NGAP listener never bound"
	case !served.Load():
		return true, "no NGAP association has been served yet"
	case shuttingDown.Load():
		return true, "shutting down"
	case associationCount() == 0:
		return false, "every NGAP association has been lost"
	default:
		return true, "serving"
	}
}

func Stop() {
	logger.NgapLog.Infoln("close SCTP server...")

	shuttingDown.Store(true)

	// The listener is nil if Listen failed, if termination beat the bind, or if Stop has
	// already run, and Stop runs before the AMF tells its peers it is unavailable, so it
	// must not end the process.
	if listener := takeListener(); listener != nil {
		if err := listener.Close(); err != nil {
			logger.NgapLog.Error(err)
			logger.NgapLog.Infof("SCTP server may not close normally.")
		}
	} else {
		logger.NgapLog.Infoln("no SCTP listener to close")
	}

	// The association is the key of this map; its value is a bool.
	connections.Range(func(key, _ interface{}) bool {
		conn, ok := key.(net.Conn)
		if !ok {
			logger.NgapLog.Errorf("connection map holds a %T key, cannot close it", key)
			return true
		}
		if err := conn.Close(); err != nil {
			logger.NgapLog.Error(err)
		}
		return true
	})

	logger.NgapLog.Infof("SCTP server closed")
}

func handleConnection(conn *sctp.SCTPConn, bufsize uint32, handler NGAPHandler) {
	buf := make([]byte, bufsize)

	defer func() {
		connections.Delete(conn)
		reportAssociationCount()

		// Notify the NGAP dispatcher that this RAN connection has closed so that
		// its AmfRan entry is removed from AmfRanPool
		if handler.HandleMessage != nil {
			handler.HandleMessage(conn, nil)
		}

		// if AMF call Stop(), then conn.Close() will return EBADF because conn has been closed inside Stop()
		if err := conn.Close(); err != nil && err != syscall.EBADF {
			logger.NgapLog.Errorf("close connection error: %+v", err)
		}
		logger.NgapLog.Infof("connection[addr: %+v] closed", conn.RemoteAddr())
	}()

	for {
		n, info, err := conn.SCTPRead(buf)
		if err != nil {
			switch err {
			case io.EOF, io.ErrUnexpectedEOF:
				logger.NgapLog.Debugf("connection[addr: %+v] closed by peer (EOF)", conn.RemoteAddr())
				return
			case syscall.EAGAIN:
				logger.NgapLog.Debugln("SCTP read timeout")
				// Timeout is set via SO_RCVTIMEO socket option, no need to reset
				continue
			case syscall.EINTR:
				logger.NgapLog.Debugf("SCTPRead interrupted: %+v", err)
				continue
			case syscall.ECONNRESET:
				logger.NgapLog.Infof("connection[addr: %+v] reset by peer", conn.RemoteAddr())
				return
			case syscall.ENOTCONN:
				logger.NgapLog.Infof("connection[addr: %+v] not connected", conn.RemoteAddr())
				return
			default:
				logger.NgapLog.Errorf("handle connection[addr: %+v] error: %+v", conn.RemoteAddr(), err)
				return
			}
		}

		// Check if this is a notification (MSG_NOTIFICATION flag)
		if info != nil && (info.Flags&sctp.MSG_NOTIFICATION) != 0 {
			logger.NgapLog.Debugf("received connection-specific SCTP notification")
			if handler.HandleNotification != nil {
				handler.HandleNotification(conn, buf[:n])
			}
			continue
		}

		// Regular message handling
		if info == nil {
			logger.NgapLog.Warnf("received SCTP message with nil SndRcvInfo, discarding packet")
			continue
		}

		if info.PPID != ngap.PPID {
			logger.NgapLog.Warnf("received SCTP PPID %d != %d (expected NGAP), discarding packet",
				info.PPID, ngap.PPID)
			continue
		}

		// Validate data length
		if n <= 0 {
			logger.NgapLog.Warnf("received empty SCTP packet, discarding")
			continue
		}

		logger.NgapLog.Debugf("read %d bytes", n)
		logger.NgapLog.Debugf("packet content: %+v", hex.Dump(buf[:n]))

		// TODO: concurrent on per-UE message
		handler.HandleMessage(conn, buf[:n])
	}
}
