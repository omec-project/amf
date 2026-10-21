// Copyright (C) 2026 Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package util

import "testing"

func TestPlmnIdStringToModels(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMcc   string
		wantMnc   string
		wantError bool
	}{
		{name: "valid two digit mnc", input: "12345", wantMcc: "123", wantMnc: "45"},
		{name: "valid three digit mnc", input: "123456", wantMcc: "123", wantMnc: "456"},
		{name: "empty", input: "", wantError: true},
		{name: "too short", input: "1234", wantError: true},
		{name: "too long", input: "1234567", wantError: true},
		{name: "non digits", input: "12a45", wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plmnID, err := PlmnIdStringToModels(tt.input)
			if tt.wantError {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if plmnID.Mcc != tt.wantMcc || plmnID.Mnc != tt.wantMnc {
				t.Fatalf("expected MCC/MNC %s/%s, got %s/%s", tt.wantMcc, tt.wantMnc, plmnID.Mcc, plmnID.Mnc)
			}
		})
	}
}

func FuzzSeparateAmfId(f *testing.F) {
	for _, seed := range []string{"cafe00", "010203", "", "zzzzzz", "ffffff", "12345"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, amfId string) {
		regionId, _, _, err := SeparateAmfId(amfId)
		if err == nil && len(amfId) != 6 {
			t.Fatalf("SeparateAmfId(%q) accepted an AMF ID that is not 6 characters", amfId)
		}
		if err == nil && regionId != amfId[:2] {
			t.Fatalf("SeparateAmfId(%q) regionId = %q, want %q", amfId, regionId, amfId[:2])
		}
	})
}

func FuzzPlmnIdStringToModels(f *testing.F) {
	for _, seed := range []string{"12345", "123456", "", "1234", "1234567", "12a45", "०१२३४"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, plmnId string) {
		plmn, err := PlmnIdStringToModels(plmnId)
		if err != nil {
			return
		}
		if len(plmn.Mcc) != 3 || (len(plmn.Mnc) != 2 && len(plmn.Mnc) != 3) {
			t.Fatalf("PlmnIdStringToModels(%q) = MCC %q MNC %q, want 3-digit MCC and 2-3 digit MNC", plmnId, plmn.Mcc, plmn.Mnc)
		}
		if plmn.Mcc+plmn.Mnc != plmnId {
			t.Fatalf("PlmnIdStringToModels(%q) did not round-trip: %q%q", plmnId, plmn.Mcc, plmn.Mnc)
		}
	})
}

func FuzzTACConfigToModels(f *testing.F) {
	for _, seed := range []string{"1", "4294967295", "4294967296", "-1", "", "abc"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, tac string) {
		if hex := TACConfigToModels(tac); hex != "" && len(hex) < 6 {
			t.Fatalf("TACConfigToModels(%q) = %q, want at least 6 hex digits", tac, hex)
		}
	})
}
