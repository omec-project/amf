// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import (
	"testing"
	"time"
)

// TestKeepaliveConfigFromOperatorSettings covers what an operator can write, including
// what they can write wrongly.
//
// The defaults are deliberately not asserted as numbers here: an unset timer is passed
// through as zero and resolved by x2x3, so writing 60s in this test would create a
// second place where the specification's value lives — the thing this arrangement
// exists to avoid. What is asserted is that nothing is disabled and nothing is
// invented.
func TestKeepaliveConfigFromOperatorSettings(t *testing.T) {
	enabled, disabled := true, false

	for _, tc := range []struct {
		name         string
		cfg          Config
		wantDisabled bool
		wantP1       time.Duration
		wantP2       time.Duration
	}{
		{
			name: "nothing configured runs the mechanism at the specification's own timers",
			cfg:  Config{},
		},
		{
			name:   "timers as configured",
			cfg:    Config{X2X3KeepaliveTimeP1: "10s", X2X3KeepaliveTimeP2: testKeepaliveP2},
			wantP1: 10 * time.Second, wantP2: 30 * time.Second,
		},
		{
			name: "explicitly enabled is the same as unset",
			cfg:  Config{X2X3KeepaliveEnabled: &enabled},
		},
		{
			name:         "explicitly disabled, for a mediation function that never acknowledges",
			cfg:          Config{X2X3KeepaliveEnabled: &disabled},
			wantDisabled: true,
		},
		{
			name: "an unparseable timer falls back rather than guessing",
			cfg:  Config{X2X3KeepaliveTimeP1: "sixty seconds"},
		},
		{
			// The pair that reads as harmless and would disconnect every connection
			// before the keepalive that keeps it is even sent.
			name: "TIME_P2 below TIME_P1 falls back to both defaults",
			cfg:  Config{X2X3KeepaliveTimeP1: "60s", X2X3KeepaliveTimeP2: testKeepaliveP2},
		},
		{
			name:         "a disabled mechanism stays disabled through a fallback",
			cfg:          Config{X2X3KeepaliveEnabled: &disabled, X2X3KeepaliveTimeP2: "1s"},
			wantDisabled: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A nil reporter is the pre-ADMF case and must not panic.
			got := keepaliveConfig(tc.cfg, nil)

			if got.Disabled != tc.wantDisabled {
				t.Errorf("Disabled = %v, want %v", got.Disabled, tc.wantDisabled)
			}
			if got.TimeP1 != tc.wantP1 {
				t.Errorf("TimeP1 = %s, want %s", got.TimeP1, tc.wantP1)
			}
			if got.TimeP2 != tc.wantP2 {
				t.Errorf("TimeP2 = %s, want %s", got.TimeP2, tc.wantP2)
			}
			if err := got.Validate(); err != nil {
				t.Errorf("the configuration handed to the mechanism is invalid: %v", err)
			}
		})
	}
}
