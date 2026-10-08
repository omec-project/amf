// SPDX-FileCopyrightText: 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package nasconv

import (
	"reflect"
	"testing"

	"github.com/omec-project/nas/v2/nasMessage"
	"github.com/omec-project/nas/v2/nasType"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
)

func TestSnssaiToNas(t *testing.T) {
	got := SnssaiToNas(models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")})
	if want := []uint8{0x04, 0x01, 0x01, 0x02, 0x03}; !reflect.DeepEqual(got, want) {
		t.Fatalf("SnssaiToNas() = %x, want %x", got, want)
	}
}

func TestRequestedNssaiToModels(t *testing.T) {
	input := &nasType.RequestedNSSAI{Iei: nasMessage.RegistrationRequestRequestedNSSAIType, Len: 5, Buffer: []uint8{0x04, 0x01, 0x01, 0x02, 0x03}}
	got, err := RequestedNssaiToModels(input)
	if err != nil {
		t.Fatalf("RequestedNssaiToModels() error = %v", err)
	}
	if len(got) != 1 || got[0].ServingSnssai.GetSst() != 1 || got[0].ServingSnssai.GetSd() != "010203" {
		t.Fatalf("RequestedNssaiToModels() = %#v", got)
	}
}

func TestRequestedNssaiToModelsRejectsMalformedLengths(t *testing.T) {
	for _, input := range []*nasType.RequestedNSSAI{
		{Len: 10, Buffer: []uint8{0x01, 0x01}},
		{Len: 3, Buffer: []uint8{0x08, 0x01, 0x02}},
		{Len: 2, Buffer: []uint8{0x04, 0x01, 0x01, 0x02, 0x03}},
	} {
		if _, err := RequestedNssaiToModels(input); err == nil {
			t.Fatalf("RequestedNssaiToModels(%x) succeeded", input.Buffer)
		}
	}
}

func TestGutiToStringUpstreamCases(t *testing.T) {
	for _, tc := range []struct {
		buf  []byte
		want string
	}{
		{[]byte{1, 2, 3}, ""},
		{[]byte{0xf1, 0x12, 0x93, 0x11, 0x22, 0x33, 1, 2, 3, 4, 5}, "21311922330102030405"},
	} {
		_, got := GutiToString(tc.buf)
		if got != tc.want {
			t.Fatalf("GutiToString(%x) = %q, want %q", tc.buf, got, tc.want)
		}
	}
}
