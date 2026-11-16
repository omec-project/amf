// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd.
//
// SPDX-License-Identifier: Apache-2.0
//

package nfregistration

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/omec-project/amf/consumer"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/nfConfigApi"
)

// The registration logic is tested in openapi/nrfregistration. These tests check that the AMF wiring
// reaches the right consumer functions.

type ctxKey struct{}

func waitFor(t *testing.T, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal(msg)
}

func startService(t *testing.T, ctx context.Context) chan<- []nfConfigApi.AccessAndMobility {
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	ch := make(chan []nfConfigApi.AccessAndMobility)
	go func() { defer close(done); StartNfRegistrationService(ctx, ch) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return ch
}

func TestStartNfRegistrationService_NonEmptyConfigRegistersThenEmptyConfigDeregisters(t *testing.T) {
	origRegister, origDeregister := consumer.SendRegisterNFInstance, consumer.SendDeregisterNFInstance
	t.Cleanup(func() {
		consumer.SendRegisterNFInstance, consumer.SendDeregisterNFInstance = origRegister, origDeregister
	})
	var registeredWith atomic.Value
	var registerCtxValue atomic.Value
	var deregistered atomic.Int32
	consumer.SendRegisterNFInstance = func(ctx context.Context, cfg []nfConfigApi.AccessAndMobility) (*models.NFProfile, string, error) {
		registerCtxValue.Store(ctx.Value(ctxKey{}))
		registeredWith.Store(cfg)
		return models.NewNFProfileWithDefaults(), "", nil
	}
	consumer.SendDeregisterNFInstance = func(context.Context) error {
		deregistered.Add(1)
		return nil
	}

	ch := startService(t, context.WithValue(context.Background(), ctxKey{}, "marker"))

	cfg := []nfConfigApi.AccessAndMobility{{PlmnId: nfConfigApi.PlmnId{Mcc: "208", Mnc: "93"}}}
	ch <- cfg
	waitFor(t, func() bool { return registeredWith.Load() != nil }, "SendRegisterNFInstance was not called")
	if got := registeredWith.Load().([]nfConfigApi.AccessAndMobility); len(got) != 1 || got[0].PlmnId.Mcc != "208" {
		t.Errorf("registered with %+v", got)
	}
	if registerCtxValue.Load() != "marker" {
		t.Errorf("the register context does not carry the service context values: %v", registerCtxValue.Load())
	}

	ch <- []nfConfigApi.AccessAndMobility{}
	waitFor(t, func() bool { return deregistered.Load() == 1 }, "SendDeregisterNFInstance was not called for an empty config")

	// the service keeps running and registers again when the config returns
	registeredWith.Store([]nfConfigApi.AccessAndMobility(nil))
	ch <- cfg
	waitFor(t, func() bool { return len(registeredWith.Load().([]nfConfigApi.AccessAndMobility)) == 1 },
		"the AMF did not register again after the config returned")
}

func TestDeregisterNF_PassesContextToConsumer(t *testing.T) {
	orig := consumer.SendDeregisterNFInstance
	t.Cleanup(func() { consumer.SendDeregisterNFInstance = orig })
	var called atomic.Int32
	var got atomic.Value
	consumer.SendDeregisterNFInstance = func(ctx context.Context) error {
		got.Store(ctx.Value(ctxKey{}))
		called.Add(1)
		return nil
	}

	DeregisterNF(context.WithValue(context.Background(), ctxKey{}, "marker"))
	if called.Load() != 1 {
		t.Errorf("SendDeregisterNFInstance calls = %d, want 1", called.Load())
	}
	if got.Load() != "marker" {
		t.Errorf("DeregisterNF did not pass its context to the consumer: %v", got.Load())
	}
}

func TestHeartbeatUsesSendUpdateNFInstance(t *testing.T) {
	origRegister, origUpdate := consumer.SendRegisterNFInstance, consumer.SendUpdateNFInstance
	t.Cleanup(func() { consumer.SendRegisterNFInstance, consumer.SendUpdateNFInstance = origRegister, origUpdate })
	var updated atomic.Int32
	profile := models.NewNFProfileWithDefaults()
	profile.SetHeartBeatTimer(1)
	consumer.SendRegisterNFInstance = func(context.Context, []nfConfigApi.AccessAndMobility) (*models.NFProfile, string, error) {
		return profile, "", nil
	}
	consumer.SendUpdateNFInstance = func(context.Context, []models.PatchItem) (*models.NFProfile, *models.ProblemDetails, error) {
		updated.Add(1)
		return profile, nil, nil
	}

	ch := startService(t, context.Background())
	ch <- []nfConfigApi.AccessAndMobility{{PlmnId: nfConfigApi.PlmnId{Mcc: "208", Mnc: "93"}}}
	waitFor(t, func() bool { return updated.Load() >= 1 }, "heartbeat did not call SendUpdateNFInstance")
}

// The heartbeat holds the registration service's mutex while it runs, so an NRF that does not answer must not
// keep it for good: the context the service passes is cancelled by DeregisterNF, and has to reach the request.
func TestHeartbeatContextEndsAHeartbeatThatIsNotAnswered(t *testing.T) {
	origRegister, origUpdate, origDeregister := consumer.SendRegisterNFInstance, consumer.SendUpdateNFInstance, consumer.SendDeregisterNFInstance
	t.Cleanup(func() {
		consumer.SendRegisterNFInstance, consumer.SendUpdateNFInstance, consumer.SendDeregisterNFInstance = origRegister, origUpdate, origDeregister
	})
	profile := models.NewNFProfileWithDefaults()
	profile.SetHeartBeatTimer(1)
	started := make(chan struct{}, 1)
	var cancelled atomic.Bool
	consumer.SendRegisterNFInstance = func(context.Context, []nfConfigApi.AccessAndMobility) (*models.NFProfile, string, error) {
		return profile, "", nil
	}
	consumer.SendUpdateNFInstance = func(ctx context.Context, _ []models.PatchItem) (*models.NFProfile, *models.ProblemDetails, error) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-ctx.Done() // the NRF never answers
		cancelled.Store(true)
		return nil, nil, ctx.Err()
	}
	consumer.SendDeregisterNFInstance = func(context.Context) error { return nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	ch := make(chan []nfConfigApi.AccessAndMobility)
	go func() { defer close(done); StartNfRegistrationService(ctx, ch) }()
	t.Cleanup(func() { cancel(); <-done })

	ch <- []nfConfigApi.AccessAndMobility{*nfConfigApi.NewAccessAndMobilityWithDefaults()}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the heartbeat was not sent")
	}

	finished := make(chan struct{})
	go func() { DeregisterNF(context.Background()); close(finished) }()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("DeregisterNF is blocked by a heartbeat the NRF does not answer")
	}
	if !cancelled.Load() {
		t.Error("the heartbeat request did not see its context cancelled")
	}
}
