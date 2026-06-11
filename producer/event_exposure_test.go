// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
)

func TestNewAmfEventReportHandlesContinuousModeWithoutOptionalLimits(t *testing.T) {
	ue := &context.AmfUe{
		Supi:                   "imsi-208930000000001",
		EventSubscriptionsInfo: make(map[string]*context.AmfUeEventSubscription),
	}
	subscriptionID := "sub-1"
	mode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
	extSubscription := models.NewExtAmfEventSubscription(
		[]models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
		"http://callback.example.test",
		"corr-id",
		"nf-id",
	)
	extSubscription.Options = mode
	ue.EventSubscriptionsInfo[subscriptionID] = &context.AmfUeEventSubscription{
		Timestamp:         time.Now().UTC(),
		EventSubscription: extSubscription,
	}

	report, ok := NewAmfEventReport(ue, models.AMFEVENTTYPE_LOCATION_REPORT, subscriptionID)
	if !ok {
		t.Fatal("expected report to be generated")
	}
	if !report.State.GetActive() {
		t.Fatal("expected continuous subscription without limits to stay active")
	}
	if report.State.HasRemainDuration() {
		t.Fatal("expected remainDuration to be omitted when expiry is not set")
	}
	if report.State.HasRemainReports() {
		t.Fatal("expected remainReports to be omitted when maxReports is not set")
	}
}
