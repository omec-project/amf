// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package consumer

import (
	"testing"

	amf_context "github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
)

func TestServingNetworkPlmnIDUsesTaiWhenPresent(t *testing.T) {
	ue := &amf_context.AmfUe{}
	ue.Tai.PlmnId = models.PlmnId{Mcc: "315", Mnc: "010"}

	servedGuami := models.Guami{
		PlmnId: models.PlmnIdNid{Mcc: "208", Mnc: "93"},
	}

	plmnID := servingNetworkPlmnID(ue, servedGuami)
	if plmnID == nil {
		t.Fatal("expected serving PLMN to be allocated")
		return
	}
	if plmnID.Mcc != "315" || plmnID.Mnc != "010" {
		t.Fatalf("expected TAI PLMN 315/010, got %s/%s", plmnID.Mcc, plmnID.Mnc)
		return
	}
}

func TestServingNetworkPlmnIDFallsBackToGuami(t *testing.T) {
	ue := &amf_context.AmfUe{}
	servedGuami := models.Guami{
		PlmnId: models.PlmnIdNid{Mcc: "208", Mnc: "93"},
	}

	plmnID := servingNetworkPlmnID(ue, servedGuami)
	if plmnID == nil {
		t.Fatal("expected serving PLMN to be allocated")
		return
	}
	if plmnID.Mcc != "208" || plmnID.Mnc != "93" {
		t.Fatalf("expected GUAMI PLMN 208/93, got %s/%s", plmnID.Mcc, plmnID.Mnc)
		return
	}
}
