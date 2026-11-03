// Copyright (C) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/openapi/v2/models"
)

func TestAmfRanFindByRanIDMixedPool(t *testing.T) {
	gnb := &AmfRan{RanPresent: RanPresentGNbId, RanId: &models.GlobalRanNodeId{GNbId: &models.GNbId{GNBValue: "000102"}}}
	ngeNbValue, n3iwfValue := "MacroNGeNB-00001", "0a0b"
	ngeNb := &AmfRan{RanPresent: RanPresentNgeNbId, RanId: &models.GlobalRanNodeId{NgeNbId: &ngeNbValue}}
	n3iwf := &AmfRan{RanPresent: RanPresentN3IwfId, RanId: &models.GlobalRanNodeId{N3IwfId: &n3iwfValue}}
	noID := &AmfRan{RanPresent: RanPresentGNbId}
	// same node IDs as above, registered under a different PLMN
	otherPlmn := models.PlmnId{Mcc: "999", Mnc: "99"}
	gnbOther := &AmfRan{RanPresent: RanPresentGNbId, RanId: &models.GlobalRanNodeId{PlmnId: otherPlmn, GNbId: &models.GNbId{GNBValue: "000102"}}}
	ngeNbOther := &AmfRan{RanPresent: RanPresentNgeNbId, RanId: &models.GlobalRanNodeId{PlmnId: otherPlmn, NgeNbId: &ngeNbValue}}
	n3iwfOther := &AmfRan{RanPresent: RanPresentN3IwfId, RanId: &models.GlobalRanNodeId{PlmnId: otherPlmn, N3IwfId: &n3iwfValue}}

	self := &AMFContext{}
	for i, r := range []*AmfRan{noID, gnbOther, ngeNbOther, n3iwfOther, gnb, ngeNb, n3iwf} {
		self.AmfRanPool.Store(i, r)
	}

	ngeNbTarget, n3iwfTarget := ngeNbValue, n3iwfValue
	plmn := models.PlmnId{Mcc: "208", Mnc: "93"}
	for _, r := range []*AmfRan{gnb, ngeNb, n3iwf} {
		r.RanId.PlmnId = plmn
	}
	tests := []struct {
		name   string
		target models.GlobalRanNodeId
		want   *AmfRan
	}{
		{"gNB", models.GlobalRanNodeId{PlmnId: plmn, GNbId: &models.GNbId{GNBValue: "000102"}}, gnb},
		{"ng-eNB target does not dereference gNB ID", models.GlobalRanNodeId{PlmnId: plmn, NgeNbId: &ngeNbTarget}, ngeNb},
		{"N3IWF target does not dereference gNB ID", models.GlobalRanNodeId{PlmnId: plmn, N3IwfId: &n3iwfTarget}, n3iwf},
		{"gNB ID on unregistered PLMN", models.GlobalRanNodeId{PlmnId: models.PlmnId{Mcc: "001", Mnc: "01"}, GNbId: &models.GNbId{GNBValue: "000102"}}, nil},
		{"ng-eNB ID on unregistered PLMN", models.GlobalRanNodeId{PlmnId: models.PlmnId{Mcc: "001", Mnc: "01"}, NgeNbId: &ngeNbTarget}, nil},
		{"N3IWF ID on unregistered PLMN", models.GlobalRanNodeId{PlmnId: models.PlmnId{Mcc: "001", Mnc: "01"}, N3IwfId: &n3iwfTarget}, nil},
		{"same gNB ID on other PLMN selects that RAN", models.GlobalRanNodeId{PlmnId: otherPlmn, GNbId: &models.GNbId{GNBValue: "000102"}}, gnbOther},
		{"same ng-eNB ID on other PLMN selects that RAN", models.GlobalRanNodeId{PlmnId: otherPlmn, NgeNbId: &ngeNbTarget}, ngeNbOther},
		{"same N3IWF ID on other PLMN selects that RAN", models.GlobalRanNodeId{PlmnId: otherPlmn, N3IwfId: &n3iwfTarget}, n3iwfOther},
		{"empty target", models.GlobalRanNodeId{}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := self.AmfRanFindByRanID(tt.target)
			if got != tt.want || ok != (tt.want != nil) {
				t.Fatalf("got (%p, %v), want %p", got, ok, tt.want)
			}
		})
	}
}
