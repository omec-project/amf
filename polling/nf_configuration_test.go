// Copyright (C) 2026 Intel Corporation
// SPDX-FileCopyrightText: 2025 Canonical Ltd.
// SPDX-License-Identifier: Apache-2.0

package polling

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	gojson "github.com/goccy/go-json"
	"github.com/omec-project/openapi/v2/nfConfigApi"
	"github.com/omec-project/openapi/v2/nfconfigpoll"
)

// The polling logic is tested in openapi/nfconfigpoll. These tests check the AMF wiring: endpoint,
// config type, normalization and delivery to both channels.

const accessMobilityJSON = `[{"plmnId":{"mcc":"208","mnc":"93"},"snssai":{"sst":1,"sd":"010203"},"tacs":["1","2"]}]`

func startPoller(t *testing.T, handler http.HandlerFunc, regCap, updCap int) (<-chan []nfConfigApi.AccessAndMobility, <-chan []nfConfigApi.AccessAndMobility) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	reg := make(chan []nfConfigApi.AccessAndMobility, regCap)
	upd := make(chan []nfConfigApi.AccessAndMobility, updCap)
	poller, err := newPoller(srv.URL, reg, upd, 5*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); poller.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return reg, upd
}

func serveJSON(t *testing.T, body string, polls *atomic.Int32) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if polls != nil {
			polls.Add(1)
		}
		if r.URL.Path != pollingPath {
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("write response: %v", err)
		}
	}
}

func recv(t *testing.T, ch <-chan []nfConfigApi.AccessAndMobility, what string) []nfConfigApi.AccessAndMobility {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(2 * time.Second):
		t.Fatalf("no config delivered on the %s channel", what)
		return nil
	}
}

func TestPolling_DeliversConfigToBothChannelsOnce(t *testing.T) {
	reg, upd := startPoller(t, serveJSON(t, accessMobilityJSON, nil), 4, 4)

	for name, ch := range map[string]<-chan []nfConfigApi.AccessAndMobility{"registration": reg, "context update": upd} {
		got := recv(t, ch, name)
		if len(got) != 1 || got[0].PlmnId.Mcc != "208" || got[0].PlmnId.Mnc != "93" || got[0].Snssai.Sst != 1 || len(got[0].Tacs) != 2 {
			t.Errorf("%s channel got %+v", name, got)
		}
	}
	select {
	case got := <-reg:
		t.Errorf("unchanged config delivered again: %+v", got)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPolling_RegistrationChannelIsServedBeforeContextUpdateChannel(t *testing.T) {
	// unbuffered registration channel: nothing may reach the context update channel before it is read
	reg, upd := startPoller(t, serveJSON(t, accessMobilityJSON, nil), 0, 4)

	select {
	case got := <-upd:
		t.Fatalf("context update channel got %+v before the registration channel was served", got)
	case <-time.After(50 * time.Millisecond):
	}
	recv(t, reg, "registration")
	recv(t, upd, "context update")
}

func TestPolling_EmptyInitialConfigIsNotDelivered(t *testing.T) {
	var polls atomic.Int32
	reg, upd := startPoller(t, serveJSON(t, `[]`, &polls), 4, 4)

	deadline := time.Now().Add(2 * time.Second)
	for polls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	select {
	case got := <-reg:
		t.Errorf("empty config delivered on the registration channel: %+v", got)
	case got := <-upd:
		t.Errorf("empty config delivered on the context update channel: %+v", got)
	default:
	}
}

// The poller used to normalize with goccy/go-json; nfconfigpoll uses encoding/json. Both must give the
// same config for AccessAndMobility, otherwise the change detection would differ.
func TestNormalizeMatchesGoccyJSONRoundTrip(t *testing.T) {
	sd := "010203"
	configs := [][]nfConfigApi.AccessAndMobility{
		{},
		{{PlmnId: nfConfigApi.PlmnId{Mcc: "208", Mnc: "93"}, Snssai: nfConfigApi.Snssai{Sst: 1, Sd: &sd}, Tacs: []string{"1", "2"}}},
		{{PlmnId: nfConfigApi.PlmnId{Mcc: "001", Mnc: "01"}, Snssai: nfConfigApi.Snssai{Sst: 2}}},
		{
			{PlmnId: nfConfigApi.PlmnId{Mcc: "001", Mnc: "01"}, Snssai: nfConfigApi.Snssai{Sst: 1}, Tacs: []string{}},
			{PlmnId: nfConfigApi.PlmnId{Mcc: "208", Mnc: "93"}, Snssai: nfConfigApi.Snssai{Sst: 3, Sd: &sd}, Tacs: []string{"7"}},
		},
	}
	for _, cfg := range configs {
		got, err := nfconfigpoll.JSONRoundTrip(cfg)
		if err != nil {
			t.Fatalf("JSONRoundTrip(%+v): %v", cfg, err)
		}
		data, err := gojson.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		var want []nfConfigApi.AccessAndMobility
		if err := gojson.Unmarshal(data, &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("round trips differ for %+v:\n encoding/json: %+v\n goccy/go-json: %+v", cfg, got, want)
		}
	}
}
