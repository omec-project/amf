// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"testing"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
	"go.uber.org/zap"
)

func TestServingNetworkPlmnIDUsesTaiWhenPresent(t *testing.T) {
	ue := &amf_context.AmfUe{
		GmmLog: zap.NewNop().Sugar(),
		Tai: models.Tai{
			PlmnId: models.PlmnId{Mcc: "315", Mnc: "010"},
		},
	}

	servedGuami := models.Guami{
		PlmnId: models.PlmnIdNid{Mcc: "208", Mnc: "93"},
	}

	plmnID := servingNetworkPlmnID(ue, servedGuami)
	if plmnID == nil {
		t.Fatal("expected serving PLMN to be allocated")
		return
	}
	if plmnID.Mcc != "315" || plmnID.Mnc != "010" {
		t.Fatalf("expected TAI PLMN 315/010, got %s/%s", plmnID.Mcc, plmnID.Mnc)
		return
	}
}

func TestServingNetworkPlmnIDFallsBackToGuami(t *testing.T) {
	ue := &amf_context.AmfUe{
		GmmLog: zap.NewNop().Sugar(),
	}
	servedGuami := models.Guami{
		PlmnId: models.PlmnIdNid{Mcc: "208", Mnc: "93"},
	}

	plmnID := servingNetworkPlmnID(ue, servedGuami)
	if plmnID == nil {
		t.Fatal("expected serving PLMN to be allocated")
		return
	}
	if plmnID.Mcc != "208" || plmnID.Mnc != "93" {
		t.Fatalf("expected GUAMI PLMN 208/93, got %s/%s", plmnID.Mcc, plmnID.Mnc)
		return
	}
}

const legacyHref = "http://a/legacy"

func TestConfirmationHref(t *testing.T) {
	href := func(h string) models.LinksValueSchema {
		l := models.NewLink()
		l.SetHref(h)
		return models.LinksValueSchema{Link: l}
	}
	hrefs := func(hs ...string) models.LinksValueSchema {
		arr := make([]models.Link, 0, len(hs))
		for _, h := range hs {
			l := models.NewLink()
			l.SetHref(h)
			arr = append(arr, *l)
		}
		return models.LinksValueSchema{ArrayOfLink: &arr}
	}
	tests := []struct {
		name  string
		links map[string]models.LinksValueSchema
		want  string
	}{
		{"spec key", map[string]models.LinksValueSchema{LinkKeyEapSession: href("http://a/spec")}, "http://a/spec"},
		{"legacy fallback", map[string]models.LinksValueSchema{linkKeyLegacy: href(legacyHref)}, legacyHref},
		{"nil spec link falls back", map[string]models.LinksValueSchema{LinkKeyEapSession: {}, linkKeyLegacy: href(legacyHref)}, legacyHref},
		{"empty spec href falls back", map[string]models.LinksValueSchema{LinkKeyEapSession: href(""), linkKeyLegacy: href(legacyHref)}, legacyHref},
		{"empty hrefs", map[string]models.LinksValueSchema{LinkKeyEapSession: href(""), linkKeyLegacy: href("")}, ""},
		{"array spec link", map[string]models.LinksValueSchema{LinkKeyEapSession: hrefs("", "http://a/array")}, "http://a/array"},
		{"empty array falls back", map[string]models.LinksValueSchema{LinkKeyEapSession: hrefs(), linkKeyLegacy: href(legacyHref)}, legacyHref},
		{"array legacy link", map[string]models.LinksValueSchema{linkKeyLegacy: hrefs("http://a/array")}, "http://a/array"},
		{"nil links", map[string]models.LinksValueSchema{LinkKeyEapSession: {}}, ""},
		{"no links", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := confirmationHref(tt.links, LinkKeyEapSession); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAusfRootUri(t *testing.T) {
	href := func(h string) models.LinksValueSchema {
		l := models.NewLink()
		l.SetHref(h)
		return models.LinksValueSchema{Link: l}
	}
	tests := []struct {
		name    string
		links   map[string]models.LinksValueSchema
		want    string
		wantErr bool
	}{
		{"spec key", map[string]models.LinksValueSchema{LinkKeyEapSession: href("http://ausf:8080/x/y")}, "http://ausf:8080", false},
		{"malformed spec href returns error", map[string]models.LinksValueSchema{LinkKeyEapSession: href("http://ausf:bad/eap-session"), linkKeyLegacy: href(legacyHref)}, "", true},
		{"empty spec href uses legacy", map[string]models.LinksValueSchema{LinkKeyEapSession: href(""), linkKeyLegacy: href(legacyHref)}, "http://a", false},
		{"no usable link returns error", map[string]models.LinksValueSchema{LinkKeyEapSession: href("")}, "", true},
		{"no links returns error", nil, "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ausfRootUri(tt.links, LinkKeyEapSession)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Fatalf("got (%q, %v), want (%q, err=%v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
