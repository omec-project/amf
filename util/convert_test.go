// Copyright 2026 Intel Corporation
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
