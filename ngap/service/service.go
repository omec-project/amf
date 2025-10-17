// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package service

import (
	"encoding/binary"
	"encoding/hex"
	"io"
	"math/bits"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/ishidawataru/sctp"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/ngap"
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
)

var handler NGAPHandler

var sctpConfig sctp.SocketConfig = sctp.SocketConfig{
	InitMsg: sctp.InitMsg{NumOstreams: 3, MaxInstreams: 5, MaxAttempts: 2, MaxInitTimeout: 2},
	NotificationHandler: func(notificationData []byte) error {
		// Handle notifications inline
		logger.NgapLog.Debugf("Received SCTP notification of size %d bytes", len(notificationData))
		// Parse notification type from the data
		if len(notificationData) >= 6 {
			// NotificationHeader: Type(2 bytes) + Flags(2 bytes) + Length(4 bytes)
			notificationType := binary.LittleEndian.Uint16(notificationData[0:2])
			logger.NgapLog.Debugf("Notification type: 0x%x", notificationType)

			// Call the handler's notification callback if it exists
			if handler.HandleNotification != nil {
				// We need to find the connection associated with this notification
				// For now, pass nil as conn since we can't easily get it in the callback
				handler.HandleNotification(nil, notificationData)
			}
		}
		return nil
	},
}

func Run(addresses []string, port int, h NGAPHandler) {
	// Store handler globally so notification callback can access it
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
	if listener, err := sctpConfig.Listen("sctp", addr); err != nil {
		logger.NgapLog.Errorf("failed to listen: %+v", err)
		return
	} else {
		sctpListener = listener
	}

	logger.NgapLog.Infof("Listen on %s", sctpListener.Addr())

	for {
		newConn, err := sctpListener.AcceptSCTP()
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
		if infoTmp, err := newConn.GetDefaultSentParam(); err != nil {
			logger.NgapLog.Errorf("get default sent param error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			info = infoTmp
			logger.NgapLog.Debugf("get default sent param[value: %+v]", info)
		}

		info.PPID = ngap.PPID
		if err := newConn.SetDefaultSentParam(info); err != nil {
			logger.NgapLog.Errorf("set default sent param error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugf("set default sent param[value: %+v]", info)
		}

		events := sctp.SCTP_EVENT_DATA_IO | sctp.SCTP_EVENT_SHUTDOWN | sctp.SCTP_EVENT_ASSOCIATION
		if err := newConn.SubscribeEvents(events); err != nil {
			logger.NgapLog.Errorf("failed to accept: %+v", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugln("subscribe SCTP event[DATA_IO, SHUTDOWN_EVENT, ASSOCIATION_CHANGE]")
		}

		if err := newConn.SetReadBuffer(int(readBufSize)); err != nil {
			logger.NgapLog.Errorf("set read buffer error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugf("Set read buffer to %d bytes", readBufSize)
		}

		// Set read deadline using time.Now() + timeout duration
		// readTimeout is syscall.Timeval{Sec: 2, Usec: 0}, so we convert to time.Duration
		deadline := time.Now().Add(time.Duration(readTimeout.Sec)*time.Second + time.Duration(readTimeout.Usec)*time.Microsecond)
		if err := newConn.SetReadDeadline(deadline); err != nil {
			logger.NgapLog.Errorf("set read deadline error: %+v, accept failed", err)
			if err = newConn.Close(); err != nil {
				logger.NgapLog.Errorf("close error: %+v", err)
			}
			continue
		} else {
			logger.NgapLog.Debugf("set read deadline: %+v", deadline)
		}

		logger.NgapLog.Infof("[AMF] SCTP Accept from: %s", newConn.RemoteAddr().String())
		connections.Store(newConn, newConn)

		go handleConnection(newConn, readBufSize, handler)
	}
}

func Stop() {
	logger.NgapLog.Infoln("close SCTP server...")
	if err := sctpListener.Close(); err != nil {
		logger.NgapLog.Error(err)
		logger.NgapLog.Infof("SCTP server may not close normally.")
	}

	connections.Range(func(key, value interface{}) bool {
		conn := value.(net.Conn)
		if err := conn.Close(); err != nil {
			logger.NgapLog.Error(err)
		}
		return true
	})

	logger.NgapLog.Infof("SCTP server closed")
}

func handleConnection(conn *sctp.SCTPConn, bufsize uint32, handler NGAPHandler) {
	defer func() {
		// if AMF call Stop(), then conn.Close() will return EBADF because conn has been closed inside Stop()
		if err := conn.Close(); err != nil && err != syscall.EBADF {
			logger.NgapLog.Errorf("close connection error: %+v", err)
		}
		connections.Delete(conn)
	}()

	for {
		buf := make([]byte, bufsize)

		n, info, err := conn.SCTPRead(buf)
		if err != nil {
			switch err {
			case io.EOF, io.ErrUnexpectedEOF:
				logger.NgapLog.Debugln("read EOF from client")
				return
			case syscall.EAGAIN:
				logger.NgapLog.Debugln("SCTP read timeout")
				// Update read deadline for next read
				deadline := time.Now().Add(time.Duration(readTimeout.Sec)*time.Second + time.Duration(readTimeout.Usec)*time.Microsecond)
				if setErr := conn.SetReadDeadline(deadline); setErr != nil {
					logger.NgapLog.Errorf("set read deadline error: %+v", setErr)
					return
				}
				continue
			case syscall.EINTR:
				logger.NgapLog.Debugf("SCTPRead: %+v", err)
				continue
			default:
				logger.NgapLog.Errorf("handle connection[addr: %+v] error: %+v", conn.RemoteAddr(), err)
				return
			}
		}

		// Notifications are now handled by the notification callback
		// So we only get here if we have actual data
		if info == nil || info.PPID != bits.ReverseBytes32(ngap.PPID) {
			logger.NgapLog.Warnln("received SCTP PPID != 60, discard this packet")
			continue
		}

		logger.NgapLog.Debugf("Read %d bytes", n)
		logger.NgapLog.Debugf("Packet content: %+v", hex.Dump(buf[:n]))

		// TODO: concurrent on per-UE message
		handler.HandleMessage(conn, buf[:n])
	}
}
