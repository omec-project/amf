// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

// TS 24.501 table 10.3.2 NOTE 5 names NR(MEO) and NR(GEO) and no other access. Table 10.3.1 says
// the same for the UE side in its NOTE 7; subclause 4.23.4 is the clause that routes the SMF to
// table 10.3.2 when the AMF indicates extended timers.
//
// The tempting shortcut is "is this non-terrestrial", which IsNtn already answers — and it is the
// wrong question. A LEO round trip at 600 to 1200 km is tens of milliseconds, so the base timer
// values are conformant there; extending them would delay every recovery on a constellation that
// does not need it. NR(OTHER_SAT) is unnamed by NOTE 5 and takes the base values by the letter of
// the specification.
func TestExtendedNasSmTimersFollowTheOrbitNotMerelyBeingSatellite(t *testing.T) {
	tests := []struct {
		ratType models.RatType
		want    bool
	}{
		{models.RATTYPE_NR_GEO, true},
		{models.RATTYPE_NR_MEO, true},
		{models.RATTYPE_NR_LEO, false},
		{models.RATTYPE_NR_OTHER_SAT, false},
		{models.RATTYPE_NR, false},
		{models.RATTYPE_EUTRA, false},
		{"", false},
	}

	for _, tc := range tests {
		name := string(tc.ratType)
		if name == "" {
			name = "unset"
		}
		t.Run(name, func(t *testing.T) {
			ue := &AmfUe{RatType: tc.ratType}
			if got := ue.UsesExtendedNasSmTimers(); got != tc.want {
				t.Errorf("UsesExtendedNasSmTimers() = %v, want %v for RAT type %q", got, tc.want, tc.ratType)
			}
		})
	}
}

// The two predicates must not be confused: every access this one accepts is non-terrestrial, and
// not every non-terrestrial access qualifies. Pinning the relationship catches a later change that
// redefines one in terms of the other.
func TestExtendedNasSmTimersAreASubsetOfNonTerrestrial(t *testing.T) {
	for _, ratType := range []models.RatType{
		models.RATTYPE_NR_LEO, models.RATTYPE_NR_MEO,
		models.RATTYPE_NR_GEO, models.RATTYPE_NR_OTHER_SAT,
		models.RATTYPE_NR, models.RATTYPE_EUTRA,
	} {
		ue := &AmfUe{RatType: ratType}
		if ue.UsesExtendedNasSmTimers() && !ue.IsNtn() {
			t.Errorf("%q warrants extended timers but is not non-terrestrial", ratType)
		}
	}

	leo := &AmfUe{RatType: models.RATTYPE_NR_LEO}
	if !leo.IsNtn() || leo.UsesExtendedNasSmTimers() {
		t.Error("NR(LEO) must be non-terrestrial and must not warrant extended timers; deriving one predicate from the other is the mistake this pins")
	}
}
