// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package util

import (
	"testing"

	"github.com/google/uuid"
	"github.com/omec-project/amf/factory"
)

func TestResolveRegisterIPv4(t *testing.T) {
	t.Setenv("AMF_REGISTER_IP", "10.10.0.1")
	t.Setenv("AMF_REGISTER_HOST", "amf.namespace.svc.cluster.local")
	t.Setenv("INVALID_REGISTER_IP", "http://amf")
	t.Setenv("POD_IP", "")

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "literal ip", input: "127.0.0.1", expected: "127.0.0.1"},
		{name: "ipv6 literal rejected", input: "2001:db8::1", expected: ""},
		{name: "service hostname", input: "amf", expected: "amf"},
		{name: "env var ip", input: "AMF_REGISTER_IP", expected: "10.10.0.1"},
		{name: "env var hostname", input: "AMF_REGISTER_HOST", expected: "amf.namespace.svc.cluster.local"},
		{name: "missing env var name", input: "POD_IP", expected: ""},
		{name: "invalid env var value", input: "INVALID_REGISTER_IP", expected: ""},
		{name: "invalid literal", input: "http://amf", expected: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := resolveRegisterIPv4(test.input); got != test.expected {
				t.Fatalf("resolveRegisterIPv4(%q) = %q, want %q", test.input, got, test.expected)
			}
		})
	}
}

func TestResolveStableAmfNfId(t *testing.T) {
	t.Run("uses resolved register IPv4", func(t *testing.T) {
		cfg := &factory.Configuration{Sbi: &factory.Sbi{RegisterIPv4: "amf"}}
		want := uuid.NewSHA1(uuid.NameSpaceOID, []byte("amf")).String()
		if got := resolveStableAmfNfId(cfg); got != want {
			t.Fatalf("resolveStableAmfNfId() = %q, want %q", got, want)
		}
	})

	t.Run("unresolved env placeholder falls back to random uuid", func(t *testing.T) {
		cfg := &factory.Configuration{Sbi: &factory.Sbi{RegisterIPv4: "POD_IP"}}
		unexpected := uuid.NewSHA1(uuid.NameSpaceOID, []byte("POD_IP")).String()
		got := resolveStableAmfNfId(cfg)
		if got == unexpected {
			t.Fatalf("resolveStableAmfNfId() unexpectedly hashed unresolved placeholder: %q", got)
		}
		if _, err := uuid.Parse(got); err != nil {
			t.Fatalf("resolveStableAmfNfId() returned invalid uuid %q: %v", got, err)
		}
	})
}

func TestResolveDrsmMongoURL(t *testing.T) {
	t.Run("uses legacy default when mongodb url is unset", func(t *testing.T) {
		cfg := &factory.Configuration{}
		if got := resolveDrsmMongoURL(cfg); got != defaultDrsmMongoURL {
			t.Fatalf("resolveDrsmMongoURL() = %q, want %q", got, defaultDrsmMongoURL)
		}
	})

	t.Run("uses configured mongodb url when set", func(t *testing.T) {
		cfg := &factory.Configuration{Mongodb: &factory.Mongodb{Url: "mongodb://mongodb-headless:27017/?replicaSet=rs0"}}
		if got := resolveDrsmMongoURL(cfg); got != cfg.Mongodb.Url {
			t.Fatalf("resolveDrsmMongoURL() = %q, want %q", got, cfg.Mongodb.Url)
		}
	})
}
