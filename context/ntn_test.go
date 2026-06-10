// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
//
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/ngap/v2/ngapType"
	"github.com/omec-project/openapi/v2/models"
)

func TestAmfUeIsNtn(t *testing.T) {
	tests := []struct {
		ratType models.RatType
		want    bool
	}{
		{ratType: "", want: false},
		{ratType: models.RATTYPE_NR, want: false},
		{ratType: models.RATTYPE_EUTRA, want: false},
		{ratType: models.RATTYPE_WLAN, want: false},
		{ratType: models.RATTYPE_NR_LEO, want: true},
		{ratType: models.RATTYPE_NR_MEO, want: true},
		{ratType: models.RATTYPE_NR_GEO, want: true},
		{ratType: models.RATTYPE_NR_OTHER_SAT, want: true},
	}
	for _, tc := range tests {
		t.Run(string(tc.ratType), func(t *testing.T) {
			ue := &AmfUe{RatType: tc.ratType}
			if got := ue.IsNtn(); got != tc.want {
				t.Errorf("IsNtn() with RatType=%q = %v, want %v", tc.ratType, got, tc.want)
			}
		})
	}
}

func TestAmfRanRatInformationForTAC(t *testing.T) {
	leo := &ngapType.RATInformation{Value: ngapType.RATInformationPresentNRLEO}
	geo := &ngapType.RATInformation{Value: ngapType.RATInformationPresentNRGEO}
	ran := &AmfRan{
		SupportedTAList: []SupportedTAI{
			{Tai: models.Tai{Tac: "000001"}, RatInformation: leo},
			{Tai: models.Tai{Tac: "000002"}, RatInformation: nil},
			{Tai: models.Tai{Tac: "000003"}, RatInformation: geo},
		},
	}

	tests := []struct {
		name string
		tac  string
		want *ngapType.RATInformation
	}{
		{name: "TAC with NRLEO", tac: "000001", want: leo},
		{name: "TAC with no extension", tac: "000002", want: nil},
		{name: "TAC with NRGEO", tac: "000003", want: geo},
		{name: "unknown TAC", tac: "ffffff", want: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ran.RatInformationForTAC(tc.tac)
			if got != tc.want {
				t.Errorf("RatInformationForTAC(%q) = %p, want %p", tc.tac, got, tc.want)
			}
			if got != nil && tc.want != nil && got.Value != tc.want.Value {
				t.Errorf("RatInformationForTAC(%q).Value = %v, want %v", tc.tac, got.Value, tc.want.Value)
			}
		})
	}
}
