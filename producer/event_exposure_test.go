// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
)

// setupAmfEventSubscription registers a subscription with the given event types in the AMF context
// and removes it once the test completes.
func setupAmfEventSubscription(t *testing.T, subscriptionID string, eventTypes []models.AmfEventType) {
	t.Helper()
	events := make([]models.AmfEvent, len(eventTypes))
	for i, eventType := range eventTypes {
		events[i] = models.AmfEvent{Type: eventType}
	}
	amfSelf := context.AMF_Self()
	amfSelf.NewEventSubscription(subscriptionID, &context.AMFContextEventSubscription{
		IsAnyUe: true,
		EventSubscription: models.AmfEventSubscription{
			EventList:           events,
			EventNotifyUri:      "http://callback.example.test",
			NotifyCorrelationId: "corr-id",
			NfId:                "nf-id",
		},
	})
	t.Cleanup(func() { amfSelf.DeleteEventSubscription(subscriptionID) })
}

func newEventListPatchRequest(op, path string, value *models.AmfEvent) models.ModifySubscriptionRequest {
	item := models.NewAmfUpdateEventSubscriptionItem(op, path)
	if value != nil {
		item.SetValue(*value)
	}
	items := []models.AmfUpdateEventSubscriptionItem{*item}
	return models.ArrayOfAmfUpdateEventSubscriptionItemAsModifySubscriptionRequest(&items)
}

func eventTypesOf(events []models.AmfEvent) []models.AmfEventType {
	types := make([]models.AmfEventType, len(events))
	for i, event := range events {
		types[i] = event.Type
	}
	return types
}

func TestModifyAMFEventSubscriptionProcedureAddInsertsAtRequestedIndex(t *testing.T) {
	tests := []struct {
		name    string
		initial []models.AmfEventType
		index   int
		want    []models.AmfEventType
	}{
		{
			name:    "add at beginning",
			initial: []models.AmfEventType{models.AMFEVENTTYPE_TIMEZONE_REPORT, models.AMFEVENTTYPE_ACCESS_TYPE_REPORT},
			index:   0,
			want: []models.AmfEventType{
				models.AMFEVENTTYPE_LOCATION_REPORT, models.AMFEVENTTYPE_TIMEZONE_REPORT, models.AMFEVENTTYPE_ACCESS_TYPE_REPORT,
			},
		},
		{
			name:    "add in middle",
			initial: []models.AmfEventType{models.AMFEVENTTYPE_TIMEZONE_REPORT, models.AMFEVENTTYPE_ACCESS_TYPE_REPORT},
			index:   1,
			want: []models.AmfEventType{
				models.AMFEVENTTYPE_TIMEZONE_REPORT, models.AMFEVENTTYPE_LOCATION_REPORT, models.AMFEVENTTYPE_ACCESS_TYPE_REPORT,
			},
		},
		{
			name:    "add at end",
			initial: []models.AmfEventType{models.AMFEVENTTYPE_TIMEZONE_REPORT, models.AMFEVENTTYPE_ACCESS_TYPE_REPORT},
			index:   2,
			want: []models.AmfEventType{
				models.AMFEVENTTYPE_TIMEZONE_REPORT, models.AMFEVENTTYPE_ACCESS_TYPE_REPORT, models.AMFEVENTTYPE_LOCATION_REPORT,
			},
		},
	}

	for i, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			subscriptionID := fmt.Sprintf("9001%d", i)
			setupAmfEventSubscription(t, subscriptionID, tc.initial)

			newEvent := models.AmfEvent{Type: models.AMFEVENTTYPE_LOCATION_REPORT}
			request := newEventListPatchRequest("add", fmt.Sprintf("/eventList/%d", tc.index), &newEvent)

			updated, problemDetails := ModifyAMFEventSubscriptionProcedure(subscriptionID, request)
			if problemDetails != nil {
				t.Fatalf("expected success, got problem details: %+v", problemDetails)
			}
			got := eventTypesOf(updated.Subscription.GetEventList())
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("event list = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestModifyAMFEventSubscriptionProcedureAddAppendsWithDashPath(t *testing.T) {
	subscriptionID := "90024"
	setupAmfEventSubscription(t, subscriptionID,
		[]models.AmfEventType{models.AMFEVENTTYPE_TIMEZONE_REPORT, models.AMFEVENTTYPE_ACCESS_TYPE_REPORT})

	newEvent := models.AmfEvent{Type: models.AMFEVENTTYPE_LOCATION_REPORT}
	request := newEventListPatchRequest("add", "/eventList/-", &newEvent)

	updated, problemDetails := ModifyAMFEventSubscriptionProcedure(subscriptionID, request)
	if problemDetails != nil {
		t.Fatalf("expected success, got problem details: %+v", problemDetails)
	}
	want := []models.AmfEventType{
		models.AMFEVENTTYPE_TIMEZONE_REPORT, models.AMFEVENTTYPE_ACCESS_TYPE_REPORT, models.AMFEVENTTYPE_LOCATION_REPORT,
	}
	got := eventTypesOf(updated.Subscription.GetEventList())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("event list = %v, want %v", got, want)
	}
}

func TestModifyAMFEventSubscriptionProcedureDashPathRejectedForNonAddOps(t *testing.T) {
	for _, op := range []string{"replace", "remove"} {
		t.Run(op, func(t *testing.T) {
			subscriptionID := fmt.Sprintf("9002%s", op)
			setupAmfEventSubscription(t, subscriptionID, []models.AmfEventType{models.AMFEVENTTYPE_TIMEZONE_REPORT})

			newEvent := models.AmfEvent{Type: models.AMFEVENTTYPE_LOCATION_REPORT}
			request := newEventListPatchRequest(op, "/eventList/-", &newEvent)

			updated, problemDetails := ModifyAMFEventSubscriptionProcedure(subscriptionID, request)
			if problemDetails == nil {
				t.Fatalf("expected problem details for %q op with dash path", op)
			}
			if updated != nil {
				t.Fatal("expected no updated subscription on error")
			}
		})
	}
}

func TestModifyAMFEventSubscriptionProcedureNegativeIndexReturnsError(t *testing.T) {
	subscriptionID := "90025"
	setupAmfEventSubscription(t, subscriptionID, []models.AmfEventType{models.AMFEVENTTYPE_TIMEZONE_REPORT})

	newEvent := models.AmfEvent{Type: models.AMFEVENTTYPE_LOCATION_REPORT}
	request := newEventListPatchRequest("replace", "/eventList/-1", &newEvent)

	updated, problemDetails := ModifyAMFEventSubscriptionProcedure(subscriptionID, request)
	if problemDetails == nil {
		t.Fatal("expected problem details for negative patch path index")
	}
	if updated != nil {
		t.Fatal("expected no updated subscription on error")
	}
}

func TestModifyAMFEventSubscriptionProcedureReplaceOutOfRangeReturnsError(t *testing.T) {
	subscriptionID := "90021"
	setupAmfEventSubscription(t, subscriptionID, []models.AmfEventType{models.AMFEVENTTYPE_TIMEZONE_REPORT})

	newEvent := models.AmfEvent{Type: models.AMFEVENTTYPE_LOCATION_REPORT}
	request := newEventListPatchRequest("replace", "/eventList/5", &newEvent)

	updated, problemDetails := ModifyAMFEventSubscriptionProcedure(subscriptionID, request)
	if problemDetails == nil {
		t.Fatal("expected problem details for out-of-range replace index")
	}
	if updated != nil {
		t.Fatal("expected no updated subscription on error")
	}
}

func TestModifyAMFEventSubscriptionProcedureRemoveOutOfRangeReturnsError(t *testing.T) {
	subscriptionID := "90022"
	setupAmfEventSubscription(t, subscriptionID, []models.AmfEventType{models.AMFEVENTTYPE_TIMEZONE_REPORT})

	request := newEventListPatchRequest("remove", "/eventList/5", nil)

	updated, problemDetails := ModifyAMFEventSubscriptionProcedure(subscriptionID, request)
	if problemDetails == nil {
		t.Fatal("expected problem details for out-of-range remove index")
	}
	if updated != nil {
		t.Fatal("expected no updated subscription on error")
	}
}

func TestModifyAMFEventSubscriptionProcedureUnsupportedOpReturnsError(t *testing.T) {
	subscriptionID := "90023"
	setupAmfEventSubscription(t, subscriptionID, []models.AmfEventType{models.AMFEVENTTYPE_TIMEZONE_REPORT})

	request := newEventListPatchRequest("move", "/eventList/0", nil)

	updated, problemDetails := ModifyAMFEventSubscriptionProcedure(subscriptionID, request)
	if problemDetails == nil {
		t.Fatal("expected problem details for unsupported patch operation")
	}
	if updated != nil {
		t.Fatal("expected no updated subscription on error")
	}
}

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
