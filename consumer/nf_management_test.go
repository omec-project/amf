// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"testing"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/nfConfigApi"
)

func makeTestSnssai(sst int32, sd string) nfConfigApi.Snssai {
	snssai := nfConfigApi.NewSnssai(sst)
	snssai.SetSd(sd)
	return *snssai
}

func makeAccessAndMobilityConfig() []nfConfigApi.AccessAndMobility {
	return []nfConfigApi.AccessAndMobility{
		{
			PlmnId: nfConfigApi.PlmnId{Mcc: "001", Mnc: "01"},
			Snssai: makeTestSnssai(1, "010203"),
			Tacs:   []string{"1"},
		},
	}
}

func TestGetNfProfileUsesFqdnForHostnameRegistration(t *testing.T) {
	originalConfig := factory.AmfConfig
	factory.AmfConfig = factory.Config{Configuration: &factory.Configuration{AmfId: "cafe00"}}
	defer func() {
		factory.AmfConfig = originalConfig
	}()

	amfCtx := &amf_context.AMFContext{
		NfId:         "amf-instance-id",
		RegisterIPv4: "amf",
		UriScheme:    models.URISCHEME_HTTP,
		SBIPort:      29518,
		NfService:    make(map[models.ServiceName]models.NFService),
	}
	amfCtx.InitNFService([]string{string(models.SERVICENAME_NAMF_COMM)}, "1.0.0")

	profile, err := getNfProfile(amfCtx, makeAccessAndMobilityConfig())
	if err != nil {
		t.Fatalf("getNfProfile() error = %v", err)
	}
	if profile.GetFqdn() != "amf" {
		t.Fatalf("profile fqdn = %q, want %q", profile.GetFqdn(), "amf")
	}
	if len(profile.Ipv4Addresses) != 0 {
		t.Fatalf("expected no ipv4Addresses, got %+v", profile.Ipv4Addresses)
	}
	if len(profile.NfServices) != 1 {
		t.Fatalf("expected 1 nf service, got %d", len(profile.NfServices))
	}
	service := profile.NfServices[0]
	if service.GetFqdn() != "amf" {
		t.Fatalf("service fqdn = %q, want %q", service.GetFqdn(), "amf")
	}
	if len(service.IpEndPoints) != 1 {
		t.Fatalf("expected 1 ip endpoint, got %d", len(service.IpEndPoints))
	}
	if service.IpEndPoints[0].GetIpv4Address() != "" {
		t.Fatalf("expected empty endpoint ipv4Address, got %q", service.IpEndPoints[0].GetIpv4Address())
	}
	if service.GetApiPrefix() != "http://amf:29518" {
		t.Fatalf("service apiPrefix = %q, want %q", service.GetApiPrefix(), "http://amf:29518")
	}
	if callback := profile.DefaultNotificationSubscriptions[0].CallbackUri; callback != "http://amf:29518/namf-callback/v1/n1-message-notify" {
		t.Fatalf("callbackUri = %q, want %q", callback, "http://amf:29518/namf-callback/v1/n1-message-notify")
	}
}

func TestGetNfProfileUsesIpv4AddressForLiteralRegistration(t *testing.T) {
	const registerIPv4 = "10.10.0.1"

	originalConfig := factory.AmfConfig
	factory.AmfConfig = factory.Config{Configuration: &factory.Configuration{AmfId: "cafe00"}}
	defer func() {
		factory.AmfConfig = originalConfig
	}()

	amfCtx := &amf_context.AMFContext{
		NfId:         "amf-instance-id",
		RegisterIPv4: registerIPv4,
		UriScheme:    models.URISCHEME_HTTP,
		SBIPort:      29518,
		NfService:    make(map[models.ServiceName]models.NFService),
	}
	amfCtx.InitNFService([]string{string(models.SERVICENAME_NAMF_COMM)}, "1.0.0")

	profile, err := getNfProfile(amfCtx, makeAccessAndMobilityConfig())
	if err != nil {
		t.Fatalf("getNfProfile() error = %v", err)
	}
	if profile.GetFqdn() != "" {
		t.Fatalf("profile fqdn = %q, want empty", profile.GetFqdn())
	}
	if len(profile.Ipv4Addresses) != 1 || profile.Ipv4Addresses[0] != registerIPv4 {
		t.Fatalf("profile ipv4Addresses = %+v, want [%s]", profile.Ipv4Addresses, registerIPv4)
	}
	service := profile.NfServices[0]
	if service.GetFqdn() != "" {
		t.Fatalf("service fqdn = %q, want empty", service.GetFqdn())
	}
	if service.IpEndPoints[0].GetIpv4Address() != registerIPv4 {
		t.Fatalf("endpoint ipv4Address = %q, want %q", service.IpEndPoints[0].GetIpv4Address(), registerIPv4)
	}
	if service.GetApiPrefix() != "http://"+registerIPv4+":29518" {
		t.Fatalf("service apiPrefix = %q, want %q", service.GetApiPrefix(), "http://"+registerIPv4+":29518")
	}
	if service.IpEndPoints[0].GetPort() != int32(29518) {
		t.Fatalf("endpoint port = %d, want %d", service.IpEndPoints[0].GetPort(), 29518)
	}
	if service.IpEndPoints[0].GetTransport() != models.TRANSPORTPROTOCOL_TCP {
		t.Fatalf("endpoint transport = %q, want %q", service.IpEndPoints[0].GetTransport(), models.TRANSPORTPROTOCOL_TCP)
	}
	if uri := amfCtx.GetSbiUri(); uri != "http://"+registerIPv4+":29518" {
		t.Fatalf("GetSbiUri() = %q, want %q", uri, "http://"+registerIPv4+":29518")
	}
	if amfCtx.GetIPv4Uri() != amfCtx.GetSbiUri() {
		t.Fatalf("GetIPv4Uri() = %q, want alias of %q", amfCtx.GetIPv4Uri(), amfCtx.GetSbiUri())
	}
	if amfCtx.RegisterFQDN() != "" {
		t.Fatalf("RegisterFQDN() = %q, want empty", amfCtx.RegisterFQDN())
	}
	if amfCtx.RegisterIPv4Address() != registerIPv4 {
		t.Fatalf("RegisterIPv4Address() = %q, want %q", amfCtx.RegisterIPv4Address(), registerIPv4)
	}
	_ = openapi.PtrString
}
