// SPDX-FileCopyrightText: 2026 Intel Corporation
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
		wantDetected bool
	}{
		{name: "nil → not-detected allocates false", initial: nil, detected: false, wantDetected: false},
		{name: "nil → detected allocates true", initial: nil, detected: true, wantDetected: true},
		{name: "detected → detected stays true", initial: &NtnAccessInfo{Detected: true}, detected: true, wantDetected: true},
		{name: "detected → not-detected clears", initial: &NtnAccessInfo{Detected: true}, detected: false, wantDetected: false},
		{name: "not-detected → detected sets true", initial: &NtnAccessInfo{Detected: false}, detected: true, wantDetected: true},
		{name: "not-detected → not-detected stays false", initial: &NtnAccessInfo{Detected: false}, detected: false, wantDetected: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ranUe := &RanUe{
				NtnAccess: tc.initial,
				Log:       zap.NewNop().Sugar(),
			}
			ranUe.updateNtnAccess(tc.detected)
			if ranUe.NtnAccess == nil {
				t.Fatalf("NtnAccess should always be non-nil after updateNtnAccess; got nil")
			}
			if ranUe.NtnAccess.Detected != tc.wantDetected {
				t.Errorf("Detected = %v, want %v", ranUe.NtnAccess.Detected, tc.wantDetected)
			}
		})
	}
}
