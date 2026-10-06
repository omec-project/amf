// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"fmt"
	"net/http"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/amf/context"
	"github.com/omec-project/openapi/v2/models"
)

const (
	testEventNotifyURI      = "http://callback.example.test"
	testNotifyCorrelationID = "corr-id"
	testNfID                = "nf-id"
	opReplace               = "replace"
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
			EventNotifyUri:      testEventNotifyURI,
			NotifyCorrelationId: testNotifyCorrelationID,
			NfId:                testNfID,
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
	for _, op := range []string{opReplace, "remove"} {
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
	request := newEventListPatchRequest(opReplace, "/eventList/-1", &newEvent)

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
	request := newEventListPatchRequest(opReplace, "/eventList/5", &newEvent)

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
		testEventNotifyURI,
		testNotifyCorrelationID,
		testNfID,
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

// A UE's subscription is given a list of its own when it is created. Handing it the subscription's
// own slice left the two sharing one array, and patching the subscription writes into that array:
// "replace" assigns an element in place and "remove" shifts the rest down. The UE's list would
// then change underneath whoever was reading it, with nothing ordering the two, and be persisted
// half-patched.
func TestAUeSubscriptionKeepsItsOwnEventList(t *testing.T) {
	amfSelf := context.AMF_Self()

	ue := &context.AmfUe{
		Supi:                   "imsi-208930000000077",
		EventSubscriptionsInfo: make(map[string]*context.AmfUeEventSubscription),
	}
	amfSelf.UePool.Store(ue.Supi, ue)

	t.Cleanup(func() { amfSelf.UePool.Delete(ue.Supi) })

	// Through the procedure itself: what is under test is the list the UE's subscription is built
	// with, and building one in the test would assert nothing about how the AMF builds it.
	anyUe := true
	created, problem := CreateAMFEventSubscriptionProcedure(models.AmfCreateEventSubscription{
		Subscription: models.AmfEventSubscription{
			AnyUE: &anyUe,
			EventList: []models.AmfEvent{
				{Type: models.AMFEVENTTYPE_LOCATION_REPORT},
				{Type: models.AMFEVENTTYPE_REGISTRATION_STATE_REPORT},
			},
			EventNotifyUri:      testEventNotifyURI,
			NotifyCorrelationId: testNotifyCorrelationID,
			NfId:                testNfID,
		},
	})
	if problem != nil {
		t.Fatalf("creating the subscription: %v", problem)
	}

	subscriptionID := created.GetSubscriptionId()
	t.Cleanup(func() { amfSelf.DeleteEventSubscription(subscriptionID) })

	// The stored subscription, not GetEventSubscription's: that hands back a snapshot with a list
	// of its own, which is the right thing for a reader and would hide exactly what this asserts.
	ueSubscription, held := ue.EventSubscriptionsInfo[subscriptionID]
	if !held {
		t.Fatal("the UE was given no subscription to keep a list for")
	}

	replacement := models.AmfEvent{Type: models.AMFEVENTTYPE_SUBSCRIPTION_ID_CHANGE}
	if _, patchProblem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
		newEventListPatchRequest(opReplace, "/eventList/0", &replacement)); patchProblem != nil {
		t.Fatalf("patching the subscription: %v", patchProblem)
	}

	if got := ueSubscription.EventSubscription.EventList[0].Type; got != models.AMFEVENTTYPE_LOCATION_REPORT {
		t.Errorf("the UE's first event became %v, want the %v it was created with: the UE and the subscription share one array",
			got, models.AMFEVENTTYPE_LOCATION_REPORT)
	}
}

// Modifying /options/expiry has to reach the per-UE subscription: NewAmfEventReport reads expiry
// from the UE's own options, so a report raised after the subscription is shortened must see the
// new expiry and report the subscription inactive, not keep using the expiry it was created with.
func TestModifyAMFEventSubscriptionProcedureExpiryReachesReports(t *testing.T) {
	amfSelf := context.AMF_Self()

	ue := &context.AmfUe{
		Supi:                   "imsi-208930000000088",
		EventSubscriptionsInfo: make(map[string]*context.AmfUeEventSubscription),
	}
	amfSelf.UePool.Store(ue.Supi, ue)
	t.Cleanup(func() { amfSelf.UePool.Delete(ue.Supi) })

	// Built through the procedure so the per-UE subscription is wired the way the AMF wires it.
	supi := ue.Supi
	future := time.Now().Add(time.Hour)
	mode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
	mode.SetExpiry(future)
	created, problem := CreateAMFEventSubscriptionProcedure(models.AmfCreateEventSubscription{
		Subscription: models.AmfEventSubscription{
			Supi:                &supi,
			EventList:           []models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
			EventNotifyUri:      testEventNotifyURI,
			NotifyCorrelationId: testNotifyCorrelationID,
			NfId:                testNfID,
			Options:             mode,
		},
	})
	if problem != nil {
		t.Fatalf("creating the subscription: %v", problem)
	}
	subscriptionID := created.GetSubscriptionId()
	t.Cleanup(func() { amfSelf.DeleteEventSubscription(subscriptionID) })

	if report, ok := NewAmfEventReport(ue, models.AMFEVENTTYPE_LOCATION_REPORT, subscriptionID); !ok || !report.State.GetActive() {
		t.Fatalf("a report raised before the expiry passes should be active (ok=%v, active=%v)", ok, report.State.GetActive())
	}

	past := time.Now().Add(-time.Hour)
	items := []models.AmfUpdateEventOptionItem{*models.NewAmfUpdateEventOptionItem(opReplace, "/options/expiry", past)}
	updated, patchProblem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
		models.ArrayOfAmfUpdateEventOptionItemAsModifySubscriptionRequest(&items))
	if patchProblem != nil {
		t.Fatalf("patching the expiry: %v", patchProblem)
	}
	// The response must echo the modified expiry so the consumer can confirm the change took effect.
	if got, ok := updated.Subscription.GetOptionsOk(); !ok || got.Expiry == nil || !got.Expiry.Equal(past) {
		t.Errorf("modify response options = %+v, want expiry %v", got, past)
	}

	report, ok := NewAmfEventReport(ue, models.AMFEVENTTYPE_LOCATION_REPORT, subscriptionID)
	if !ok {
		t.Fatal("expected a report to be generated after the expiry patch")
	}
	if report.State.GetActive() {
		t.Error("a report raised after the expiry is shortened to the past must be inactive: the modified expiry did not reach the UE's subscription")
	}
}

// The subscription is serialized into the response by the HTTP layer after the procedure returns,
// so a later modify must not reach back into an earlier response. An in-place "replace" patch of a
// shared event list would do exactly that unless the response carries its own copy.
func TestModifyAMFEventSubscriptionProcedureResponseSnapshotIsIndependent(t *testing.T) {
	subscriptionID := "90091"
	setupAmfEventSubscription(t, subscriptionID,
		[]models.AmfEventType{models.AMFEVENTTYPE_TIMEZONE_REPORT, models.AMFEVENTTYPE_ACCESS_TYPE_REPORT})

	first := models.AmfEvent{Type: models.AMFEVENTTYPE_LOCATION_REPORT}
	resp1, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
		newEventListPatchRequest("add", "/eventList/0", &first))
	if problem != nil {
		t.Fatalf("first patch: %v", problem)
	}
	before := resp1.Subscription.GetEventList()[0].Type

	replacement := models.AmfEvent{Type: models.AMFEVENTTYPE_SUBSCRIPTION_ID_CHANGE}
	if _, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
		newEventListPatchRequest(opReplace, "/eventList/0", &replacement)); problem != nil {
		t.Fatalf("second patch: %v", problem)
	}

	if after := resp1.Subscription.GetEventList()[0].Type; after != before {
		t.Errorf("first response's event[0] changed from %v to %v after a later modify: the response shares the subscription's event list",
			before, after)
	}
}

// A modify targeting a subscription that no longer exists must report "not found" rather than
// touching UE state. This is the user-visible half of the guard that also, after locking, rejects a
// subscription whose ID was reused by a concurrent delete/create.
func TestModifyAMFEventSubscriptionProcedureRejectsDeletedSubscription(t *testing.T) {
	subscriptionID := "90093"
	setupAmfEventSubscription(t, subscriptionID, []models.AmfEventType{models.AMFEVENTTYPE_LOCATION_REPORT})

	if problem := DeleteAMFEventSubscriptionProcedure(subscriptionID); problem != nil {
		t.Fatalf("deleting the subscription: %+v", problem)
	}

	items := []models.AmfUpdateEventOptionItem{
		*models.NewAmfUpdateEventOptionItem(opReplace, "/options/expiry", time.Now().Add(time.Hour)),
	}
	updated, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
		models.ArrayOfAmfUpdateEventOptionItemAsModifySubscriptionRequest(&items))
	if updated != nil {
		t.Error("expected no updated subscription for a deleted ID")
	}
	if problem == nil || problem.GetStatus() != http.StatusNotFound {
		t.Errorf("expected 404 not found for a deleted subscription, got %+v", problem)
	}
}

// An expiry patch targets the optional options block. On a subscription created without options the
// patch cannot take effect, so it must be rejected rather than acknowledged as applied.
func TestModifyAMFEventSubscriptionProcedureRejectsExpiryWithoutOptions(t *testing.T) {
	subscriptionID := "90097"
	// setupAmfEventSubscription registers a subscription with no options block.
	setupAmfEventSubscription(t, subscriptionID, []models.AmfEventType{models.AMFEVENTTYPE_LOCATION_REPORT})

	items := []models.AmfUpdateEventOptionItem{
		*models.NewAmfUpdateEventOptionItem(opReplace, "/options/expiry", time.Now().Add(time.Hour)),
	}
	updated, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
		models.ArrayOfAmfUpdateEventOptionItemAsModifySubscriptionRequest(&items))
	if updated != nil {
		t.Error("expected no updated subscription for an expiry patch when options are absent")
	}
	if problem == nil || problem.GetStatus() != http.StatusBadRequest {
		t.Errorf("expected 400 bad request for an expiry patch without options, got %+v", problem)
	}
}

// The AMF acts only on /options/expiry. A malformed op or a path outside the model's set is
// rejected as bad input (400); the model-valid but unimplemented paths (notifFlag,
// mutingExcInstructions) are rejected as a capability gap (501). None may change state.
func TestModifyAMFEventSubscriptionProcedureRejectsUnsupportedOptionPatch(t *testing.T) {
	amfSelf := context.AMF_Self()
	subscriptionID := "90099"
	originalExpiry := time.Now().Add(time.Hour)
	mode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
	mode.SetExpiry(originalExpiry)
	amfSelf.NewEventSubscription(subscriptionID, &context.AMFContextEventSubscription{
		IsAnyUe: true,
		EventSubscription: models.AmfEventSubscription{
			EventList:           []models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
			EventNotifyUri:      testEventNotifyURI,
			NotifyCorrelationId: testNotifyCorrelationID,
			NfId:                testNfID,
			Options:             mode,
		},
	})
	t.Cleanup(func() { amfSelf.DeleteEventSubscription(subscriptionID) })

	cases := []struct {
		name, op, path string
		wantStatus     int32
	}{
		{"malformed op on expiry", "add", "/options/expiry", http.StatusBadRequest},
		{"path outside the model", opReplace, "/options/maxReports", http.StatusBadRequest},
		{"valid but unimplemented notifFlag", opReplace, "/options/notifFlag", http.StatusNotImplemented},
		{"valid but unimplemented mutingExc", opReplace, "/options/mutingExcInstructions", http.StatusNotImplemented},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			items := []models.AmfUpdateEventOptionItem{
				*models.NewAmfUpdateEventOptionItem(tc.op, tc.path, time.Now().Add(2*time.Hour)),
			}
			updated, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
				models.ArrayOfAmfUpdateEventOptionItemAsModifySubscriptionRequest(&items))
			if updated != nil {
				t.Error("expected no updated subscription for an unsupported option patch")
			}
			if problem == nil || problem.GetStatus() != tc.wantStatus {
				t.Errorf("expected status %d, got %+v", tc.wantStatus, problem)
			}
			if got := mode.GetExpiry(); !got.Equal(originalExpiry) {
				t.Errorf("expiry = %v, want it untouched at %v: an unsupported patch overwrote it", got, originalExpiry)
			}
		})
	}
}

// The option patch array is a slice: an empty one must be rejected (not panic on indexing), and a
// list of several supported operations must apply in order so the final expiry wins rather than the
// first silently taking effect.
func TestModifyAMFEventSubscriptionProcedureOptionPatchItemCount(t *testing.T) {
	amfSelf := context.AMF_Self()
	subscriptionID := "90101"
	mode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
	mode.SetExpiry(time.Now().Add(time.Hour))
	amfSelf.NewEventSubscription(subscriptionID, &context.AMFContextEventSubscription{
		IsAnyUe: true,
		EventSubscription: models.AmfEventSubscription{
			EventList:           []models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
			EventNotifyUri:      testEventNotifyURI,
			NotifyCorrelationId: testNotifyCorrelationID,
			NfId:                testNfID,
			Options:             mode,
		},
	})
	t.Cleanup(func() { amfSelf.DeleteEventSubscription(subscriptionID) })

	t.Run("empty list rejected without panic", func(t *testing.T) {
		empty := []models.AmfUpdateEventOptionItem{}
		updated, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
			models.ArrayOfAmfUpdateEventOptionItemAsModifySubscriptionRequest(&empty))
		if updated != nil {
			t.Error("expected no update for an empty option patch list")
		}
		if problem == nil || problem.GetStatus() != http.StatusBadRequest {
			t.Errorf("expected 400 bad request for an empty option patch list, got %+v", problem)
		}
	})

	t.Run("last of multiple operations wins", func(t *testing.T) {
		first := time.Now().Add(2 * time.Hour)
		last := time.Now().Add(10 * time.Hour)
		items := []models.AmfUpdateEventOptionItem{
			*models.NewAmfUpdateEventOptionItem(opReplace, "/options/expiry", first),
			*models.NewAmfUpdateEventOptionItem(opReplace, "/options/expiry", last),
		}
		updated, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
			models.ArrayOfAmfUpdateEventOptionItemAsModifySubscriptionRequest(&items))
		if problem != nil {
			t.Fatalf("patching: %+v", problem)
		}
		if opts, ok := updated.Subscription.GetOptionsOk(); !ok || opts.Expiry == nil || !opts.Expiry.Equal(last) {
			t.Errorf("expiry = %+v, want the last operation's %v", opts, last)
		}
	})

	t.Run("one unsupported item rejects the whole patch", func(t *testing.T) {
		items := []models.AmfUpdateEventOptionItem{
			*models.NewAmfUpdateEventOptionItem(opReplace, "/options/expiry", time.Now().Add(time.Hour)),
			*models.NewAmfUpdateEventOptionItem("add", "/options/expiry", time.Now().Add(2*time.Hour)),
		}
		updated, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
			models.ArrayOfAmfUpdateEventOptionItemAsModifySubscriptionRequest(&items))
		if updated != nil {
			t.Error("expected no update when one item is unsupported")
		}
		if problem == nil || problem.GetStatus() != http.StatusBadRequest {
			t.Errorf("expected 400 bad request, got %+v", problem)
		}
	})
}

// The per-UE subscription's advertised Options.MaxReports must be separate storage from the running
// RemainReports counter. They aliased one *int32 before, so DecrementRemainReports (as reports are
// delivered) mutated the advertised value too -- a decreasing maxReports and a race against the
// non-atomic serialization of Options on UE-context export.
func TestCreateAMFEventSubscriptionProcedureMaxReportsNotAliasedToCounter(t *testing.T) {
	amfSelf := context.AMF_Self()
	ue := &context.AmfUe{
		Supi:                   "imsi-208930000000102",
		EventSubscriptionsInfo: make(map[string]*context.AmfUeEventSubscription),
	}
	amfSelf.UePool.Store(ue.Supi, ue)
	t.Cleanup(func() { amfSelf.UePool.Delete(ue.Supi) })

	supi := ue.Supi
	maxReports := int32(5)
	mode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
	mode.SetExpiry(time.Now().Add(time.Hour))
	mode.MaxReports = &maxReports
	created, problem := CreateAMFEventSubscriptionProcedure(models.AmfCreateEventSubscription{
		Subscription: models.AmfEventSubscription{
			Supi:                &supi,
			EventList:           []models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
			EventNotifyUri:      testEventNotifyURI,
			NotifyCorrelationId: testNotifyCorrelationID,
			NfId:                testNfID,
			Options:             mode,
		},
	})
	if problem != nil {
		t.Fatalf("create: %+v", problem)
	}
	id := created.GetSubscriptionId()
	t.Cleanup(func() {
		amfSelf.DeleteEventSubscription(id)
		ue.DeleteEventSubscription(id)
	})

	stored, ok := ue.EventSubscriptionsInfo[id]
	if !ok || stored.EventSubscription == nil || stored.RemainReports == nil {
		t.Fatal("the UE did not retain a continuous subscription with a counter")
	}
	opts, ok := stored.EventSubscription.GetOptionsOk()
	if !ok || opts.MaxReports == nil {
		t.Fatal("the stored subscription lost its MaxReports")
	}
	subReports(ue, id)
	if got := opts.GetMaxReports(); got != 5 {
		t.Errorf("advertised MaxReports = %d after a report was counted, want it unchanged at 5", got)
	}
	if got := *stored.RemainReports; got != 4 {
		t.Errorf("RemainReports = %d after one decrement, want 4", got)
	}
}

// The create response is serialized by the HTTP layer after the subscription is published under a
// predictable ID, so a concurrent PATCH must not reach back into it. The response therefore has to
// carry its own copy of the event list, just as the modify response does.
func TestCreateAMFEventSubscriptionProcedureResponseSnapshotIsIndependent(t *testing.T) {
	amfSelf := context.AMF_Self()
	ue := &context.AmfUe{
		Supi:                   "imsi-208930000000104",
		EventSubscriptionsInfo: make(map[string]*context.AmfUeEventSubscription),
	}
	amfSelf.UePool.Store(ue.Supi, ue)
	t.Cleanup(func() { amfSelf.UePool.Delete(ue.Supi) })

	anyUe := true
	created, problem := CreateAMFEventSubscriptionProcedure(models.AmfCreateEventSubscription{
		Subscription: models.AmfEventSubscription{
			AnyUE: &anyUe,
			EventList: []models.AmfEvent{
				{Type: models.AMFEVENTTYPE_TIMEZONE_REPORT},
				{Type: models.AMFEVENTTYPE_ACCESS_TYPE_REPORT},
			},
			EventNotifyUri:      testEventNotifyURI,
			NotifyCorrelationId: testNotifyCorrelationID,
			NfId:                testNfID,
		},
	})
	if problem != nil {
		t.Fatalf("create: %+v", problem)
	}
	id := created.GetSubscriptionId()
	t.Cleanup(func() {
		amfSelf.DeleteEventSubscription(id)
		ue.DeleteEventSubscription(id)
	})

	before := created.Subscription.GetEventList()[0].Type

	// A later modify patches the published subscription's event list in place.
	replacement := models.AmfEvent{Type: models.AMFEVENTTYPE_SUBSCRIPTION_ID_CHANGE}
	if _, p := ModifyAMFEventSubscriptionProcedure(id,
		newEventListPatchRequest(opReplace, "/eventList/0", &replacement)); p != nil {
		t.Fatalf("patch: %+v", p)
	}

	if after := created.Subscription.GetEventList()[0].Type; after != before {
		t.Errorf("create response event[0] changed from %v to %v after a later modify: the response shares the subscription's event list",
			before, after)
	}
}

// An immediate subscription whose first report is already inactive (expiry in the past) is a
// one-time report, not a standing subscription: Create must remove it -- context entry and per-UE
// entry -- through the lock-and-identity-checked path rather than a bare context delete.
func TestCreateAMFEventSubscriptionProcedureImmediateInactiveSubscriptionIsRemoved(t *testing.T) {
	amfSelf := context.AMF_Self()
	ue := &context.AmfUe{
		Supi:                   "imsi-208930000000101",
		EventSubscriptionsInfo: make(map[string]*context.AmfUeEventSubscription),
	}
	amfSelf.UePool.Store(ue.Supi, ue)
	t.Cleanup(func() { amfSelf.UePool.Delete(ue.Supi) })

	supi := ue.Supi
	immediate := true
	mode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
	mode.SetExpiry(time.Now().Add(-time.Hour)) // already expired -> immediate report is inactive
	created, problem := CreateAMFEventSubscriptionProcedure(models.AmfCreateEventSubscription{
		Subscription: models.AmfEventSubscription{
			Supi:                &supi,
			EventList:           []models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT, ImmediateFlag: &immediate}},
			EventNotifyUri:      testEventNotifyURI,
			NotifyCorrelationId: testNotifyCorrelationID,
			NfId:                testNfID,
			Options:             mode,
		},
	})
	if problem != nil {
		t.Fatalf("create: %+v", problem)
	}
	id := created.GetSubscriptionId()
	t.Cleanup(func() {
		amfSelf.DeleteEventSubscription(id)
		ue.DeleteEventSubscription(id)
	})

	if _, ok := amfSelf.FindEventSubscription(id); ok {
		t.Error("an immediate inactive subscription should be removed from the context")
	}
	if _, ok := ue.GetEventSubscription(id); ok {
		t.Error("an immediate inactive subscription should leave no per-UE entry")
	}
}

// A modify request that carries neither patch list changes nothing, so it must be rejected rather
// than acknowledged as a successful update.
func TestModifyAMFEventSubscriptionProcedureRejectsEmptyRequest(t *testing.T) {
	subscriptionID := "90098"
	setupAmfEventSubscription(t, subscriptionID, []models.AmfEventType{models.AMFEVENTTYPE_LOCATION_REPORT})

	updated, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID, models.ModifySubscriptionRequest{})
	if updated != nil {
		t.Error("expected no updated subscription for a modify request with no patch list")
	}
	if problem == nil || problem.GetStatus() != http.StatusBadRequest {
		t.Errorf("expected 400 bad request for an empty modify request, got %+v", problem)
	}
}

// A modify that looked up a subscription, then had its ID reused by a concurrent delete/create
// before it locked, must reject the stale request instead of applying the PATCH to the replacement.
// modifyAfterLookupHook interposes exactly that delete/recreate in the window the post-lock identity
// check guards; without that check this modify would succeed and corrupt the reused UE entry.
func TestModifyAMFEventSubscriptionProcedureRejectsReusedIDAfterLookup(t *testing.T) {
	amfSelf := context.AMF_Self()
	subscriptionID := "90096"

	withOptions := func(expiry time.Time) *models.ExtAmfEventSubscription {
		ext := models.NewExtAmfEventSubscription(
			[]models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
			testEventNotifyURI, testNotifyCorrelationID, testNfID,
		)
		mode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
		mode.SetExpiry(expiry)
		ext.SetOptions(*mode)
		return ext
	}

	ue := &context.AmfUe{
		Supi:                   "imsi-208930000000100",
		EventSubscriptionsInfo: make(map[string]*context.AmfUeEventSubscription),
	}
	amfSelf.UePool.Store(ue.Supi, ue)
	t.Cleanup(func() { amfSelf.UePool.Delete(ue.Supi) })

	// What the modify looks up: a subscription covering this UE, with options so it would reach the
	// per-UE propagation if the identity check were missing.
	originalMode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
	originalMode.SetExpiry(time.Now().Add(time.Hour))
	original := &context.AMFContextEventSubscription{
		UeSupiList: []string{ue.Supi},
		EventSubscription: models.AmfEventSubscription{
			EventList:           []models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
			EventNotifyUri:      testEventNotifyURI,
			NotifyCorrelationId: testNotifyCorrelationID,
			NfId:                testNfID,
			Options:             originalMode,
		},
	}
	amfSelf.NewEventSubscription(subscriptionID, original)
	t.Cleanup(func() { amfSelf.DeleteEventSubscription(subscriptionID) })

	// The UE entry that the reused ID now belongs to: a stale PATCH must never touch its expiry.
	replacementExpiry := time.Now().Add(48 * time.Hour)
	ue.SetEventSubscription(subscriptionID, &context.AmfUeEventSubscription{EventSubscription: withOptions(replacementExpiry)})

	// Interpose once, after the modify has looked up `original` but before it locks: free the ID and
	// store a different object under it -- a concurrent delete followed by a create that reuses it.
	fired := false
	modifyAfterLookupHook = func() {
		if fired {
			return
		}
		fired = true
		amfSelf.DeleteEventSubscription(subscriptionID)
		amfSelf.NewEventSubscription(subscriptionID, &context.AMFContextEventSubscription{
			EventSubscription: models.AmfEventSubscription{
				EventList:           []models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
				EventNotifyUri:      testEventNotifyURI,
				NotifyCorrelationId: testNotifyCorrelationID,
				NfId:                testNfID,
				Options:             originalMode,
			},
		})
	}
	t.Cleanup(func() { modifyAfterLookupHook = nil })

	items := []models.AmfUpdateEventOptionItem{
		*models.NewAmfUpdateEventOptionItem(opReplace, "/options/expiry", time.Now().Add(-time.Hour)),
	}
	updated, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
		models.ArrayOfAmfUpdateEventOptionItemAsModifySubscriptionRequest(&items))

	if updated != nil {
		t.Error("a modify whose ID was reused after lookup must not report success")
	}
	if problem == nil || problem.GetStatus() != http.StatusNotFound {
		t.Errorf("expected 404 not found for a reused ID, got %+v", problem)
	}
	// The UE entry belongs to the replacement now; the stale PATCH must not have rewritten its expiry.
	got, ok := ue.GetEventSubscription(subscriptionID)
	if !ok || got.EventSubscription == nil {
		t.Fatal("the UE lost its subscription entry")
	}
	if opts, ok := got.EventSubscription.GetOptionsOk(); !ok || !opts.GetExpiry().Equal(replacementExpiry) {
		t.Errorf("reused-ID UE expiry = %v, want untouched %v: the stale modify wrote to the reused entry",
			opts.GetExpiry(), replacementExpiry)
	}
}

// A second delete of the same ID must not free it twice (the ID is returned to the generator for
// reuse on the first delete): the post-lock revalidation, or the initial lookup, has to reject it.
func TestDeleteAMFEventSubscriptionProcedureIsIdempotent(t *testing.T) {
	subscriptionID := "90094"
	setupAmfEventSubscription(t, subscriptionID, []models.AmfEventType{models.AMFEVENTTYPE_LOCATION_REPORT})

	if problem := DeleteAMFEventSubscriptionProcedure(subscriptionID); problem != nil {
		t.Fatalf("first delete: %+v", problem)
	}
	problem := DeleteAMFEventSubscriptionProcedure(subscriptionID)
	if problem == nil || problem.GetStatus() != http.StatusNotFound {
		t.Errorf("expected 404 not found on second delete, got %+v", problem)
	}
}

// Concurrent create/modify/delete on reused subscription IDs exercise the per-subscription lock and
// its deadlock-free ordering (subscription then UE). Run under -race, this must complete without a
// deadlock, panic, or data race whichever way the modify and delete interleave.
func TestEventSubscriptionConcurrentModifyDeleteIsSafe(t *testing.T) {
	amfSelf := context.AMF_Self()
	ue := &context.AmfUe{
		Supi:                   "imsi-208930000000099",
		EventSubscriptionsInfo: make(map[string]*context.AmfUeEventSubscription),
	}
	amfSelf.UePool.Store(ue.Supi, ue)
	t.Cleanup(func() { amfSelf.UePool.Delete(ue.Supi) })
	supi := ue.Supi

	for range 50 {
		mode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
		mode.SetExpiry(time.Now().Add(time.Hour))
		created, problem := CreateAMFEventSubscriptionProcedure(models.AmfCreateEventSubscription{
			Subscription: models.AmfEventSubscription{
				Supi:                &supi,
				EventList:           []models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
				EventNotifyUri:      testEventNotifyURI,
				NotifyCorrelationId: testNotifyCorrelationID,
				NfId:                testNfID,
				Options:             mode,
			},
		})
		if problem != nil {
			t.Fatalf("create: %+v", problem)
		}
		id := created.GetSubscriptionId()

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			items := []models.AmfUpdateEventOptionItem{
				*models.NewAmfUpdateEventOptionItem(opReplace, "/options/expiry", time.Now().Add(2*time.Hour)),
			}
			_, _ = ModifyAMFEventSubscriptionProcedure(id,
				models.ArrayOfAmfUpdateEventOptionItemAsModifySubscriptionRequest(&items))
		}()
		go func() {
			defer wg.Done()
			_ = DeleteAMFEventSubscriptionProcedure(id)
		}()
		wg.Wait()

		// Leave nothing behind for the next iteration regardless of how the two interleaved.
		_ = DeleteAMFEventSubscriptionProcedure(id)
		ue.DeleteEventSubscription(id)
	}
}

// Concurrent modify requests on one subscription mutate and snapshot the single shared object. Run
// under -race, this trips the detector unless those mutations and snapshots are serialized.
func TestModifyAMFEventSubscriptionProcedureConcurrentPatchesAreRaceFree(t *testing.T) {
	subscriptionID := "90092"
	amfSelf := context.AMF_Self()
	mode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
	mode.SetExpiry(time.Now().Add(time.Hour))
	amfSelf.NewEventSubscription(subscriptionID, &context.AMFContextEventSubscription{
		IsAnyUe: true,
		EventSubscription: models.AmfEventSubscription{
			EventList:           []models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
			EventNotifyUri:      testEventNotifyURI,
			NotifyCorrelationId: testNotifyCorrelationID,
			NfId:                testNfID,
			Options:             mode,
		},
	})
	t.Cleanup(func() { amfSelf.DeleteEventSubscription(subscriptionID) })

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			items := []models.AmfUpdateEventOptionItem{
				*models.NewAmfUpdateEventOptionItem(opReplace, "/options/expiry", time.Now().Add(time.Duration(i)*time.Minute)),
			}
			updated, problem := ModifyAMFEventSubscriptionProcedure(subscriptionID,
				models.ArrayOfAmfUpdateEventOptionItemAsModifySubscriptionRequest(&items))
			if problem != nil {
				t.Errorf("modify returned problem: %+v", problem)
				return
			}
			// Read the response's nested options: without the snapshot this races the next modify.
			if opts, ok := updated.Subscription.GetOptionsOk(); ok {
				_ = opts.GetExpiry()
			}
		}(i)
	}
	wg.Wait()
}
