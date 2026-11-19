// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package lawfulintercept

import "testing"

// TestKeepaliveWindowIsReadNotGuessed covers the parse of one keepalive timer.
//
// Empty is the clause default, and must not be confused with a value this element could
// not read: reading that as zero too would turn a mistyped duration into a silently
// different timer on an element that otherwise looks healthy.
func TestKeepaliveWindowIsReadNotGuessed(t *testing.T) {
	for _, tc := range []struct {
		value   string
		wantErr bool
	}{
		{value: ""},                    // the clause default
		{value: "30s"},                 //
		{value: "5m"},                  //
		{value: "30", wantErr: true},   // the typo this exists for
		{value: "5min", wantErr: true}, // and the other one
		{value: "30 s", wantErr: true}, //
		{value: "0s", wantErr: true},   // a window that cannot mean what was asked
		{value: "-5m", wantErr: true},  //
	} {
		got, err := parseKeepaliveTimer("keepaliveTimeout", tc.value)
		if (err != nil) != tc.wantErr {
			t.Errorf("parseKeepaliveTimer(%q) error = %v, wantErr %v", tc.value, err, tc.wantErr)
		}
		if err != nil && got != 0 {
			t.Errorf("parseKeepaliveTimer(%q) returned %s alongside an error; a value that "+
				"could not be read must not become a window", tc.value, got)
		}
	}
}
