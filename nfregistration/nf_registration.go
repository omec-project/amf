// SPDX-FileCopyrightText: 2025 Canonical Ltd
// SPDX-FileCopyrightText: 2024 Intel Corporation
// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package nfregistration

import (
	"context"
	"sync"
	"time"

	"github.com/omec-project/amf/consumer"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/openapi/nfConfigApi"
)

var (
	keepAliveTimer      *time.Timer
	keepAliveTimerMutex sync.Mutex
	registerCtxMutex    sync.Mutex
	afterFunc           = time.AfterFunc
)

const (
	defaultHeartbeatTimer int32 = 60
	retryTime                   = 10 * time.Second
)

// StartNfRegistrationService starts the registration service. If the new config is empty, the NF
// deregisters from the NRF. Else, it registers to the NRF. It cancels registerCancel to ensure
// that only one registration process runs at the time.
func StartNfRegistrationService(ctx context.Context, accessAndMobilityConfigChan <-chan []nfConfigApi.AccessAndMobility) {
	var registerCancel context.CancelFunc
	var registerCtx context.Context
	logger.NrfRegistrationLog.Infoln("started NF registration to NRF service")
	for {
		select {
		case <-ctx.Done():
			if registerCancel != nil {
				registerCancel()
			}
			logger.NrfRegistrationLog.Infoln("NF registration service shutting down")
			return
		case newAccessAndMobilityConfig := <-accessAndMobilityConfigChan:
			// Cancel current sync if running
			if registerCancel != nil {
				logger.NrfRegistrationLog.Infoln("NF registration context cancelled")
				registerCancel()
			}

			if len(newAccessAndMobilityConfig) == 0 {
				logger.NrfRegistrationLog.Debugln("Access and Mobility config is empty. AMF will deregister")
				DeregisterNF(ctx)
				return
			}
			logger.NrfRegistrationLog.Debugln("AMF will update registration")
			registerCtx, registerCancel = context.WithCancel(context.Background())
			// Create new cancellable context for this registration
			go registerNF(registerCtx, newAccessAndMobilityConfig)
		}
	}
}

// registerNF sends a RegisterNFInstance. If it fails, it keeps retrying, until the context is cancelled by StartNfRegistrationService
var registerNF = func(registerCtx context.Context, newAccessAndMobilityConfig []nfConfigApi.AccessAndMobility) {
	registerCtxMutex.Lock()
	defer registerCtxMutex.Unlock()
	interval := 0 * time.Millisecond
	for {
		select {
		case <-registerCtx.Done():
			logger.NrfRegistrationLog.Infoln("no-op. Registration context was cancelled")
			return
		case <-time.After(interval):
			nfProfile, _, err := consumer.SendRegisterNFInstance(registerCtx, newAccessAndMobilityConfig)
			if err != nil {
				logger.NrfRegistrationLog.Errorf("register AMF instance to NRF failed. Will retry in %s", retryTime.String())
				interval = retryTime
				continue
			}
			logger.NrfRegistrationLog.Infoln("register AMF instance to NRF with updated profile succeeded")
			startKeepAliveTimer(registerCtx, nfProfile.HeartBeatTimer, newAccessAndMobilityConfig)
			return
		}
	}
}

// heartbeatNF is the callback function, this is called when keepalivetimer elapsed.
// It sends a Update NF instance to the NRF. If it fails, it tries to register again.
// keepAliveTimer is restarted at the end.
func heartbeatNF(registerCtx context.Context, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) {
	keepAliveTimerMutex.Lock()
	if keepAliveTimer == nil {
		keepAliveTimerMutex.Unlock()
		logger.NrfRegistrationLog.Infoln("heartbeat timer has been stopped, heartbeat will not be sent to NRF")
		return
	}
	keepAliveTimerMutex.Unlock()

	patchItem := []models.PatchItem{
		{
			Op:    "replace",
			Path:  "/nfStatus",
			Value: "REGISTERED",
		},
	}
	nfProfile, problemDetails, err := consumer.SendUpdateNFInstance(patchItem)

	if shouldRegister(problemDetails, err) {
		logger.NrfRegistrationLog.Debugln("NF heartbeat failed. Trying to register again")
		nfProfile, _, err = consumer.SendRegisterNFInstance(registerCtx, accessAndMobilityConfig)
		if err != nil {
			logger.NrfRegistrationLog.Errorln("register AMF instance error:", err.Error())
		} else {
			logger.NrfRegistrationLog.Infoln("register AMF instance to NRF with updated profile succeeded")
		}
	} else {
		logger.NrfRegistrationLog.Debugln("AMF update NF instance (heartbeat) succeeded")
	}
	startKeepAliveTimer(registerCtx, nfProfile.HeartBeatTimer, accessAndMobilityConfig)
}

func shouldRegister(problemDetails *models.ProblemDetails, err error) bool {
	if problemDetails != nil {
		logger.NrfRegistrationLog.Warnf("AMF update NF instance (heartbeat) problem details: %+v", problemDetails)
		return true
	}
	if err != nil {
		logger.NrfRegistrationLog.Warnln("AMF update NF instance (heartbeat) error:", err.Error())
		return true
	}
	return false
}

var DeregisterNF = func(registerCtx context.Context) {
	keepAliveTimerMutex.Lock()
	stopKeepAliveTimer()
	keepAliveTimerMutex.Unlock()
	err := consumer.SendDeregisterNFInstance(registerCtx)
	if err != nil {
		logger.NrfRegistrationLog.Warnln("deregister instance from NRF error:", err.Error())
		return
	}
	logger.NrfRegistrationLog.Infoln("deregister instance from NRF successful")
}

func startKeepAliveTimer(registerCtx context.Context, profileHeartbeatTimer int32, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) {
	keepAliveTimerMutex.Lock()
	defer keepAliveTimerMutex.Unlock()
	stopKeepAliveTimer()
	heartbeatTimer := defaultHeartbeatTimer
	if profileHeartbeatTimer > 0 {
		heartbeatTimer = profileHeartbeatTimer
	}
	heartbeatFunction := func() { heartbeatNF(registerCtx, accessAndMobilityConfig) }
	// AfterFunc starts timer and waits for keepAliveTimer to elapse and then calls heartbeatNF function
	keepAliveTimer = afterFunc(time.Duration(heartbeatTimer)*time.Second, heartbeatFunction)
	logger.NrfRegistrationLog.Debugf("started heartbeat timer: %v sec", heartbeatTimer)
}

func stopKeepAliveTimer() {
	if keepAliveTimer != nil {
		keepAliveTimer.Stop()
		keepAliveTimer = nil
		logger.NrfRegistrationLog.Debugln("stopped heartbeat timer")
	}
}
