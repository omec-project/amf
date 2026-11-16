// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/amf/factory"
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
	factory.AmfConfig = factory.Config{Configuration: &factory.Configuration{AmfId: testAmfID}}
	defer func() {
		factory.AmfConfig = originalConfig
	}()

	amfCtx := &amf_context.AMFContext{
		NfId:         testNfInstanceID,
		RegisterIPv4: nfName,
		UriScheme:    models.URISCHEME_HTTP,
		SBIPort:      29518,
		NfService:    make(map[models.ServiceName]models.NFService),
	}
	amfCtx.InitNFService([]string{string(models.SERVICENAME_NAMF_COMM)}, "1.0.0")

	profile, err := getNfProfile(amfCtx, makeAccessAndMobilityConfig())
	if err != nil {
		t.Fatalf("getNfProfile() error = %v", err)
	}
	if profile.GetFqdn() != nfName {
		t.Fatalf("profile fqdn = %q, want %q", profile.GetFqdn(), nfName)
	}
	if len(profile.Ipv4Addresses) != 0 {
		t.Fatalf("expected no ipv4Addresses, got %+v", profile.Ipv4Addresses)
	}
	nfServiceList := profile.GetNfServiceList()
	if len(nfServiceList) != 1 {
		t.Fatalf("expected 1 nf service, got %d", len(nfServiceList))
	}
	service := nfServiceList["0"]
	if service.GetFqdn() != nfName {
		t.Fatalf("service fqdn = %q, want %q", service.GetFqdn(), nfName)
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
	factory.AmfConfig = factory.Config{Configuration: &factory.Configuration{AmfId: testAmfID}}
	defer func() {
		factory.AmfConfig = originalConfig
	}()

	amfCtx := &amf_context.AMFContext{
		NfId:         testNfInstanceID,
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
	service := profile.GetNfServiceList()["0"]
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
}

// SendUpdateNFInstance is bounded by the context it is given, so that an NRF that never answers cannot hold up
// whoever waits for it.
func TestSendUpdateNFInstanceHonoursItsContext(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-release }))
	t.Cleanup(func() { close(release); srv.Close() })

	self := amf_context.AMF_Self()
	origUri, origId := self.NrfUri, self.NfId
	self.NrfUri, self.NfId = srv.URL, "4947a69a-f61b-4bc1-b9da-47c9c5d14b64"
	t.Cleanup(func() { self.NrfUri, self.NfId = origUri, origId })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	patch := []models.PatchItem{{Op: models.PATCHOPERATION_REPLACE, Path: "/nfStatus", Value: models.NFSTATUS_REGISTERED}}

	returned := make(chan error, 1)
	go func() {
		_, _, err := SendUpdateNFInstance(ctx, patch)
		returned <- err
	}()
	select {
	case err := <-returned:
		if err == nil {
			t.Error("expected an error for a request that was cut off")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SendUpdateNFInstance did not return after its context ended")
	}
}
