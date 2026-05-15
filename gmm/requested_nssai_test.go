// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package gmm

import (
	"testing"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"go.uber.org/zap"
)

func TestRebuildAllowedNssaiFromRequestedDeduplicates(t *testing.T) {
	ue := &context.AmfUe{
		GmmLog:          zap.NewNop().Sugar(),
		AllowedNssai:    map[models.AccessType][]models.AllowedSnssai{},
		SubscribedNssai: []models.SubscribedSnssai{{SubscribedSnssai: models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}}},
	}
	ue.AllowedNssai[models.ACCESSTYPE__3_GPP_ACCESS] = []models.AllowedSnssai{
		{AllowedSnssai: models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}},
		{AllowedSnssai: models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}},
	}

	requested := []models.MappingOfSnssai{
		{ServingSnssai: models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}},
		{ServingSnssai: models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}},
		{ServingSnssai: models.Snssai{Sst: 1, Sd: openapi.PtrString("010203")}},
	}

	needSliceSelection := rebuildAllowedNssaiFromRequested(ue, models.ACCESSTYPE__3_GPP_ACCESS, requested)
	if needSliceSelection {
		t.Fatal("expected subscribed requested NSSAI to avoid NSSF selection")
	}
	if got := len(ue.AllowedNssai[models.ACCESSTYPE__3_GPP_ACCESS]); got != 1 {
		t.Fatalf("expected one deduplicated allowed NSSAI entry, got %d", got)
	}
	allowed := ue.AllowedNssai[models.ACCESSTYPE__3_GPP_ACCESS][0].AllowedSnssai
	if allowed.GetSst() != 1 || allowed.GetSd() != "010203" {
		t.Fatalf("unexpected allowed NSSAI entry: %+v", allowed)
	}
}
