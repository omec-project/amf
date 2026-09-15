// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

// models.Tai carries Nid as a pointer, so == compares pointer identity for it and two TAIs
// with equal values built from separate copies never match. UpdateLocation compares the UE's
// stored TAI - which GetTai hands back as a deep copy - against the one it just built, so in
// an SNPN deployment every location update looked like a change and set LocationChanged.
const testNid = "ffffffffff0"

func TestSameTaiComparesValuesNotPointers(t *testing.T) {
	nidA, nidB := testNid, testNid
	other := "ffffffffff1"

	plmn := models.PlmnId{Mcc: "208", Mnc: "93"}

	tests := []struct {
		name string
		a, b models.Tai
		same bool
	}{
		{
			name: "equal values, distinct Nid pointers",
			a:    models.Tai{PlmnId: plmn, Tac: "000001", Nid: &nidA},
			b:    models.Tai{PlmnId: plmn, Tac: "000001", Nid: &nidB},
			same: true,
		},
		{
			name: "different Nid value",
			a:    models.Tai{PlmnId: plmn, Tac: "000001", Nid: &nidA},
			b:    models.Tai{PlmnId: plmn, Tac: "000001", Nid: &other},
			same: false,
		},
		{
			name: "one Nid absent",
			a:    models.Tai{PlmnId: plmn, Tac: "000001", Nid: &nidA},
			b:    models.Tai{PlmnId: plmn, Tac: "000001"},
			same: false,
		},
		{
			name: "both Nid absent",
			a:    models.Tai{PlmnId: plmn, Tac: "000001"},
			b:    models.Tai{PlmnId: plmn, Tac: "000001"},
			same: true,
		},
		{
			name: "different tracking area code",
			a:    models.Tai{PlmnId: plmn, Tac: "000001"},
			b:    models.Tai{PlmnId: plmn, Tac: "000002"},
			same: false,
		},
		{
			name: "different PLMN",
			a:    models.Tai{PlmnId: plmn, Tac: "000001"},
			b:    models.Tai{PlmnId: models.PlmnId{Mcc: "208", Mnc: "94"}, Tac: "000001"},
			same: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameTai(tt.a, tt.b); got != tt.same {
				t.Errorf("sameTai() = %v, want %v", got, tt.same)
			}
		})
	}
}

// These pin sameTai itself. Neither of UpdateLocation's two call sites is exercised by any test
// here or elsewhere, and a call-site test could not tell the fixed code from the unfixed while
// nothing assigns Tai.Nid: that path builds a TAI with a nil Nid at both ends, where == and
// sameTai agree. The day an SNPN deployment populates it is the day such a test can distinguish
// them, and the day a partial revert of one of the two sites would start to matter.

// The case the defect was about, through the accessor the caller actually uses: a UE whose
// stored TAI is handed back as a deep copy still compares equal to the TAI it was set from.
func TestAStoredTaiStillMatchesTheOneItWasSetFrom(t *testing.T) {
	nid := testNid
	tai := models.Tai{PlmnId: models.PlmnId{Mcc: "208", Mnc: "93"}, Tac: "000001", Nid: &nid}

	ue := &AmfUe{}
	ue.init()
	ue.SetTai(tai)

	if !sameTai(ue.GetTai(), tai) {
		t.Error("a TAI read back through GetTai does not match the one it was set from, so an " +
			"unchanged location reads as a change")
	}

	if ue.GetTai() == tai {
		t.Error("GetTai returned a value that is == to the original, so this test is no longer " +
			"exercising the deep copy the accessor makes")
	}
}
