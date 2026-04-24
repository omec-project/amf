// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"go.uber.org/zap"
)

func TestRanUeUpdateNtnAccess(t *testing.T) {
	tests := []struct {
		name         string
		initial      *NtnAccessInfo
		detected     bool
		wantPresent  bool
		wantDetected bool
	}{
		{
			name:        "nil + not-detected stays nil",
			initial:     nil,
			detected:    false,
			wantPresent: false,
		},
		{
			name:         "nil + detected allocates true",
			initial:      nil,
			detected:     true,
			wantPresent:  true,
			wantDetected: true,
		},
		{
			name:         "detected + detected stays true",
			initial:      &NtnAccessInfo{Detected: true},
			detected:     true,
			wantPresent:  true,
			wantDetected: true,
		},
		{
			name:         "detected + not-detected stays true (sticky)",
			initial:      &NtnAccessInfo{Detected: true},
			detected:     false,
			wantPresent:  true,
			wantDetected: true,
		},
		{
			name:         "not-detected + detected sets true",
			initial:      &NtnAccessInfo{Detected: false},
			detected:     true,
			wantPresent:  true,
			wantDetected: true,
		},
		{
			// Unreachable in production (we never allocate with Detected=false),
			// but documents the function's contract: sticky means a !detected
			// signal never modifies state.
			name:         "not-detected + not-detected stays false",
			initial:      &NtnAccessInfo{Detected: false},
			detected:     false,
			wantPresent:  true,
			wantDetected: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ranUe := &RanUe{
				NtnAccess: tc.initial,
				Log:       zap.NewNop().Sugar(),
			}
			ranUe.updateNtnAccess(tc.detected)

			if got := ranUe.NtnAccess != nil; got != tc.wantPresent {
				t.Fatalf("NtnAccess presence: got %v, want %v", got, tc.wantPresent)
			}
			if !tc.wantPresent {
				return
			}
			if ranUe.NtnAccess.Detected != tc.wantDetected {
				t.Errorf("Detected = %v, want %v", ranUe.NtnAccess.Detected, tc.wantDetected)
			}
		})
	}
}
