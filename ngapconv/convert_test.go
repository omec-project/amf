// SPDX-FileCopyrightText: 2026 Intel Corporation
// Copyright 2019 Communication Service/Software Laboratory, National Chiao Tung University (free5gc.org)
// SPDX-License-Identifier: Apache-2.0

package ngapconv

import (
	"testing"

	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func TestPlmnIdToModelsUpstreamCases(t *testing.T) {
	for _, tc := range []struct {
		value   []byte
		wantErr bool
	}{
		{[]byte{0x02, 0xf8, 0x39}, false},
		{[]byte{0x13, 0x20, 0x06}, false},
		{[]byte{0x02, 0xf8}, true},
		{[]byte{0xa2, 0xf8, 0x39}, true},
		{[]byte{0x02, 0xf8, 0xa9}, true},
	} {
		_, err := PlmnIdToModels(ngapType.PLMNIdentity{Value: tc.value})
		if (err != nil) != tc.wantErr {
			t.Fatalf("PlmnIdToModels(%x) error = %v", tc.value, err)
		}
	}
}

func TestSNssaiToModelsUpstreamCases(t *testing.T) {
	for _, tc := range []struct {
		value   ngapType.SNSSAI
		wantErr bool
	}{
		{ngapType.SNSSAI{SST: ngapType.SST{Value: []byte{1}}}, false},
		{ngapType.SNSSAI{SST: ngapType.SST{Value: []byte{1, 2}}}, true},
		{ngapType.SNSSAI{SST: ngapType.SST{Value: []byte{1}}, SD: &ngapType.SD{Value: []byte{1}}}, true},
	} {
		_, err := SNssaiToModels(tc.value)
		if (err != nil) != tc.wantErr {
			t.Fatalf("SNssaiToModels(%#v) error = %v", tc.value, err)
		}
	}
}

func TestTaiToModelsRejectsMalformedPlmn(t *testing.T) {
	_, err := TaiToModels(ngapType.TAI{PLMNIdentity: ngapType.PLMNIdentity{Value: []byte{0x02, 0xf8}}, TAC: ngapType.TAC{Value: []byte{0, 0, 7}}})
	if err == nil {
		t.Fatal("TaiToModels accepted an invalid PLMN")
	}
}

func TestTraceDataToNgap(t *testing.T) {
	trace := *models.NewTraceData("20893-a1b2c3", models.TRACEDEPTH_MINIMUM, "01", "01")
	got, err := TraceDataToNgap(trace, "abcd")
	if err != nil {
		t.Fatalf("TraceDataToNgap() error = %v", err)
	}
	if want := []byte{0x02, 0xf8, 0x39, 0xa1, 0xb2, 0xc3, 0xab, 0xcd}; string(got.NGRANTraceID.Value) != string(want) {
		t.Fatalf("TraceDataToNgap() NGRANTraceID = %x, want %x", got.NGRANTraceID.Value, want)
	}
}

func TestTraceDataToNgapRejectsMalformedTraceReference(t *testing.T) {
	for _, tc := range []struct {
		traceRef string
		trsr     string
	}{
		{"invalid", "abcd"},
		{"2089300-a1b2c3", "abcd"},
		{"20893-a1b2", "abcd"},
		{"20893-a1b2cg", "abcd"},
		{"20893-a1b2c3", "abcg"},
	} {
		trace := *models.NewTraceData(tc.traceRef, models.TRACEDEPTH_MINIMUM, "01", "01")
		if _, err := TraceDataToNgap(trace, tc.trsr); err == nil {
			t.Fatalf("TraceDataToNgap(%q, %q) succeeded", tc.traceRef, tc.trsr)
		}
	}
}

func TestPlmnRoundTrip(t *testing.T) {
	want := models.PlmnId{Mcc: "208", Mnc: "93"}
	got, err := PlmnIdToModels(PlmnIdToNgap(want))
	if err != nil {
		t.Fatalf("PlmnIdToModels() error = %v", err)
	}
	if got != want {
		t.Fatalf("PLMN round trip = %#v, want %#v", got, want)
	}
}

func TestSNssaiRoundTrip(t *testing.T) {
	want := models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}
	got, err := SNssaiToModels(SNssaiToNgap(want))
	if err != nil {
		t.Fatalf("SNssaiToModels() error = %v", err)
	}
	if got.GetSst() != want.GetSst() || got.GetSd() != want.GetSd() {
		t.Fatalf("S-NSSAI round trip = %#v", got)
	}
}
