// SPDX-FileCopyrightText: 2025 Canonical Ltd.
//
// SPDX-License-Identifier: Apache-2.0
//
/*
 * NRF Registration Unit Testcases
 *
 */
package nfregistration

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omec-project/amf/consumer"
	"github.com/omec-project/openapi/models"
	"github.com/omec-project/openapi/nfConfigApi"
)

func waitForSignal(t *testing.T, signal <-chan struct{}, timeout time.Duration, message string) {
	t.Helper()
	timeoutTimer := time.NewTimer(timeout)
	defer timeoutTimer.Stop()

	select {
	case <-signal:
	case <-timeoutTimer.C:
		t.Fatal(message)
	}
}

func currentKeepAliveTimer() *time.Timer {
	keepAliveTimerMutex.Lock()
	defer keepAliveTimerMutex.Unlock()
	return keepAliveTimer
}

func TestNfRegistrationService_WhenEmptyConfig_ThenDeregisterNFAndStopTimer(t *testing.T) {
	isDeregisterNFCalled := false
	testCases := []struct {
		name                         string
		sendDeregisterNFInstanceMock func(ctx context.Context) error
	}{
		{
			name: "Success",
			sendDeregisterNFInstanceMock: func(ctx context.Context) error {
				isDeregisterNFCalled = true
				return nil
			},
		},
		{
			name: "ErrorInDeregisterNFInstance",
			sendDeregisterNFInstanceMock: func(ctx context.Context) error {
				isDeregisterNFCalled = true
				return errors.New("mock error")
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			keepAliveTimer = time.NewTimer(60 * time.Second)
			isRegisterNFCalled := false
			isDeregisterNFCalled = false
			originalDeregisterNF := consumer.SendDeregisterNFInstance
			originalRegisterNF := registerNF
			defer func() {
				consumer.SendDeregisterNFInstance = originalDeregisterNF
				registerNF = originalRegisterNF
				if keepAliveTimer != nil {
					keepAliveTimer.Stop()
				}
			}()

			consumer.SendDeregisterNFInstance = tc.sendDeregisterNFInstanceMock
			registerNF = func(ctx context.Context, newAccessAndMobilityConfig []nfConfigApi.AccessAndMobility) {
				isRegisterNFCalled = true
			}

			ch := make(chan []nfConfigApi.AccessAndMobility, 1)
			ctx := t.Context()
			serviceDone := make(chan struct{})
			go func() {
				defer close(serviceDone)
				StartNfRegistrationService(ctx, ch)
			}()
			ch <- []nfConfigApi.AccessAndMobility{}
			timeoutTimer := time.NewTimer(1 * time.Second)
			defer timeoutTimer.Stop()

			select {
			case <-serviceDone:
			case <-timeoutTimer.C:
				t.Fatal("timed out waiting for NF registration service to stop")
			}

			if keepAliveTimer != nil {
				t.Errorf("expected keepAliveTimer to be nil after stopKeepAliveTimer")
			}
			if !isDeregisterNFCalled {
				t.Errorf("expected SendDeregisterNFInstance to be called")
			}
			if isRegisterNFCalled {
				t.Errorf("expected registerNF not to be called")
			}
		})
	}
}

func TestNfRegistrationService_WhenConfigChanged_ThenRegisterNFSuccessAndStartTimer(t *testing.T) {
	keepAliveTimer = nil
	originalSendRegisterNFInstance := consumer.SendRegisterNFInstance
	originalRegisterNF := registerNF
	defer func() {
		consumer.SendRegisterNFInstance = originalSendRegisterNFInstance
		registerNF = originalRegisterNF
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	registrations := []nfConfigApi.AccessAndMobility{}
	var registrationsMu sync.Mutex
	registerDone := make(chan struct{})
	var registerDoneOnce sync.Once
	consumer.SendRegisterNFInstance = func(ctx context.Context, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) (models.NfProfile, string, error) {
		profile := models.NfProfile{HeartBeatTimer: 60}
		registrationsMu.Lock()
		registrations = append(registrations, accessAndMobilityConfig...)
		registrationsMu.Unlock()
		return profile, "", nil
	}
	registerNF = func(registerCtx context.Context, newAccessAndMobilityConfig []nfConfigApi.AccessAndMobility) {
		defer registerDoneOnce.Do(func() { close(registerDone) })
		originalRegisterNF(registerCtx, newAccessAndMobilityConfig)
	}

	ch := make(chan []nfConfigApi.AccessAndMobility, 1)
	ctx, cancel := context.WithCancel(t.Context())
	serviceDone := make(chan struct{})
	go func() {
		defer close(serviceDone)
		StartNfRegistrationService(ctx, ch)
	}()
	newConfig := []nfConfigApi.AccessAndMobility{
		{
			PlmnId: *nfConfigApi.NewPlmnId("001", "01"),
			Snssai: *nfConfigApi.NewSnssai(3),
			Tacs:   []string{},
		},
	}
	ch <- newConfig

	waitForSignal(t, registerDone, time.Second, "timed out waiting for NF registration to complete")
	cancel()
	waitForSignal(t, serviceDone, time.Second, "timed out waiting for NF registration service to stop")

	if currentKeepAliveTimer() == nil {
		t.Errorf("expected keepAliveTimer to be initialized by startKeepAliveTimer")
	}
	registrationsMu.Lock()
	if !reflect.DeepEqual(registrations, newConfig) {
		t.Errorf("expected %+v config, received %+v", newConfig, registrations)
	}
	registrationsMu.Unlock()
}

func TestNfRegistrationService_ConfigChanged_RetryIfRegisterNFFails(t *testing.T) {
	originalSendRegisterNFInstance := consumer.SendRegisterNFInstance
	originalRegisterNF := registerNF
	defer func() {
		consumer.SendRegisterNFInstance = originalSendRegisterNFInstance
		registerNF = originalRegisterNF
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	var called atomic.Int32
	registerAttempts := make(chan struct{}, 2)
	registerDone := make(chan struct{})
	var registerDoneOnce sync.Once
	consumer.SendRegisterNFInstance = func(ctx context.Context, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) (models.NfProfile, string, error) {
		profile := models.NfProfile{HeartBeatTimer: 60}
		called.Add(1)
		select {
		case registerAttempts <- struct{}{}:
		default:
		}
		return profile, "", errors.New("mock error")
	}
	registerNF = func(registerCtx context.Context, newAccessAndMobilityConfig []nfConfigApi.AccessAndMobility) {
		defer registerDoneOnce.Do(func() { close(registerDone) })
		originalRegisterNF(registerCtx, newAccessAndMobilityConfig)
	}

	ch := make(chan []nfConfigApi.AccessAndMobility, 1)
	ctx, cancel := context.WithCancel(t.Context())
	serviceDone := make(chan struct{})
	defer cancel()
	go func() {
		defer close(serviceDone)
		StartNfRegistrationService(ctx, ch)
	}()
	ch <- []nfConfigApi.AccessAndMobility{
		{
			PlmnId: *nfConfigApi.NewPlmnId("001", "01"),
			Snssai: *nfConfigApi.NewSnssai(3),
			Tacs:   []string{},
		},
	}

	waitForSignal(t, registerAttempts, retryTime+time.Second, "timed out waiting for first register attempt")
	waitForSignal(t, registerAttempts, retryTime+time.Second, "timed out waiting for retry register attempt")
	cancel()
	waitForSignal(t, registerDone, time.Second, "timed out waiting for register loop to stop")
	waitForSignal(t, serviceDone, time.Second, "timed out waiting for NF registration service to stop")

	if called.Load() < 2 {
		t.Errorf("expected to retry register to NRF")
	}
	t.Logf("Tried %v times", called.Load())
}

func TestNfRegistrationService_WhenConfigChanged_ThenPreviousRegistrationIsCancelled(t *testing.T) {
	originalRegisterNf := registerNF
	defer func() {
		registerNF = originalRegisterNf
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	var registrationsMu sync.Mutex
	var registrations []struct {
		ctx    context.Context
		config []nfConfigApi.AccessAndMobility
	}
	registrationStarted := make(chan struct{}, 2)
	registerNF = func(registerCtx context.Context, newAccessAndMobilityConfig []nfConfigApi.AccessAndMobility) {
		registrationsMu.Lock()
		registrations = append(registrations, struct {
			ctx    context.Context
			config []nfConfigApi.AccessAndMobility
		}{registerCtx, newAccessAndMobilityConfig})
		registrationsMu.Unlock()
		registrationStarted <- struct{}{}
		<-registerCtx.Done() // Wait until registration is cancelled
	}

	ch := make(chan []nfConfigApi.AccessAndMobility, 1)
	ctx, cancel := context.WithCancel(t.Context())
	serviceDone := make(chan struct{})
	defer cancel()
	go func() {
		defer close(serviceDone)
		StartNfRegistrationService(ctx, ch)
	}()
	firstConfig := []nfConfigApi.AccessAndMobility{
		{
			PlmnId: *nfConfigApi.NewPlmnId("001", "01"),
			Snssai: *nfConfigApi.NewSnssai(3),
			Tacs:   []string{},
		},
	}
	ch <- firstConfig

	waitForSignal(t, registrationStarted, time.Second, "timed out waiting for first registration")
	registrationsMu.Lock()
	if len(registrations) != 1 {
		t.Errorf("expected one registration to the NRF")
	}
	registrationsMu.Unlock()

	secondConfig := []nfConfigApi.AccessAndMobility{
		{
			PlmnId: *nfConfigApi.NewPlmnId("001", "02"),
			Snssai: *nfConfigApi.NewSnssai(3),
			Tacs:   []string{},
		},
	}
	ch <- secondConfig
	waitForSignal(t, registrationStarted, time.Second, "timed out waiting for second registration")
	registrationsMu.Lock()
	if len(registrations) != 2 {
		t.Errorf("expected 2 registrations to the NRF")
	}

	select {
	case <-registrations[0].ctx.Done():
		// expected
	default:
		t.Errorf("expected first registration context to be cancelled")
	}

	select {
	case <-registrations[1].ctx.Done():
		t.Error("second registration context should not be cancelled")
	default:
		// expected
	}

	if !reflect.DeepEqual(registrations[0].config, firstConfig) {
		t.Errorf("expected %+v config, received %+v", firstConfig, registrations)
	}
	if !reflect.DeepEqual(registrations[1].config, secondConfig) {
		t.Errorf("expected %+v config, received %+v", secondConfig, registrations)
	}
	registrationsMu.Unlock()

	cancel()
	waitForSignal(t, serviceDone, time.Second, "timed out waiting for NF registration service to stop")
}

func TestHeartbeatNF_Success(t *testing.T) {
	keepAliveTimer = time.NewTimer(60 * time.Second)
	calledRegister := false
	originalSendRegisterNFInstance := consumer.SendRegisterNFInstance
	originalSendUpdateNFInstance := consumer.SendUpdateNFInstance
	defer func() {
		consumer.SendRegisterNFInstance = originalSendRegisterNFInstance
		consumer.SendUpdateNFInstance = originalSendUpdateNFInstance
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	consumer.SendUpdateNFInstance = func(patchItem []models.PatchItem) (models.NfProfile, *models.ProblemDetails, error) {
		return models.NfProfile{}, nil, nil
	}
	consumer.SendRegisterNFInstance = func(ctx context.Context, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) (models.NfProfile, string, error) {
		calledRegister = true
		profile := models.NfProfile{HeartBeatTimer: 60}
		return profile, "", nil
	}
	accessAndMobilityConfig := []nfConfigApi.AccessAndMobility{}
	heartbeatNF(t.Context(), accessAndMobilityConfig)

	if calledRegister {
		t.Errorf("expected registerNF to be called on error")
	}
	if keepAliveTimer == nil {
		t.Errorf("expected keepAliveTimer to be initialized by startKeepAliveTimer")
	}
}

func TestHeartbeatNF_WhenNfUpdateFails_ThenNfRegistersIsCalled(t *testing.T) {
	keepAliveTimer = time.NewTimer(60 * time.Second)
	calledRegister := false
	originalSendRegisterNFInstance := consumer.SendRegisterNFInstance
	originalSendUpdateNFInstance := consumer.SendUpdateNFInstance
	defer func() {
		consumer.SendRegisterNFInstance = originalSendRegisterNFInstance
		consumer.SendUpdateNFInstance = originalSendUpdateNFInstance
		if keepAliveTimer != nil {
			keepAliveTimer.Stop()
		}
	}()

	consumer.SendUpdateNFInstance = func(patchItem []models.PatchItem) (models.NfProfile, *models.ProblemDetails, error) {
		return models.NfProfile{}, nil, errors.New("mock error")
	}

	consumer.SendRegisterNFInstance = func(ctx context.Context, accessAndMobilityConfig []nfConfigApi.AccessAndMobility) (models.NfProfile, string, error) {
		profile := models.NfProfile{HeartBeatTimer: 60}
		calledRegister = true
		return profile, "", nil
	}

	accessAndMobilityConfig := []nfConfigApi.AccessAndMobility{}
	heartbeatNF(t.Context(), accessAndMobilityConfig)

	if !calledRegister {
		t.Errorf("expected registerNF to be called on error")
	}
	if keepAliveTimer == nil {
		t.Errorf("expected keepAliveTimer to be initialized by startKeepAliveTimer")
	}
}

func TestStartKeepAliveTimer_UsesProfileTimerOnlyWhenGreaterThanZero(t *testing.T) {
	testCases := []struct {
		name             string
		profileTime      int32
		expectedDuration time.Duration
	}{
		{
			name:             "Profile heartbeat time is zero, use default time",
			profileTime:      0,
			expectedDuration: 60 * time.Second,
		},
		{
			name:             "Profile heartbeat time is smaller than zero, use default time",
			profileTime:      -5,
			expectedDuration: 60 * time.Second,
		},
		{
			name:             "Profile heartbeat time is greater than zero, use profile time",
			profileTime:      15,
			expectedDuration: 15 * time.Second,
		},
		{
			name:             "Profile heartbeat time is greater than default time, use profile time",
			profileTime:      90,
			expectedDuration: 90 * time.Second,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			keepAliveTimer = time.NewTimer(25 * time.Second)
			defer func() {
				if keepAliveTimer != nil {
					keepAliveTimer.Stop()
				}
			}()
			var capturedDuration time.Duration

			afterFunc = func(d time.Duration, _ func()) *time.Timer {
				capturedDuration = d
				return time.NewTimer(25 * time.Second)
			}
			defer func() { afterFunc = time.AfterFunc }()

			startKeepAliveTimer(t.Context(), tc.profileTime, nil)
			if tc.expectedDuration != capturedDuration {
				t.Errorf("expected %v duration, got %v", tc.expectedDuration, capturedDuration)
			}
		})
	}
}
