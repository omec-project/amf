// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"
	"time"

	"github.com/omec-project/openapi/v2/models"
)

// An any-UE/group subscription gives every UE its own wrapper but one shared *ExtAmfEventSubscription.
// SetEventSubscriptionExpiry must repoint the UE it updates at a private copy, so changing one UE's
// expiry leaves every other UE's options -- read under a different lock -- untouched.
func TestSetEventSubscriptionExpiryDoesNotTouchOtherUEs(t *testing.T) {
	const id = "shared-sub"
	original := time.Now().Add(time.Hour)

	shared := models.NewExtAmfEventSubscription(
		[]models.AmfEvent{{Type: models.AMFEVENTTYPE_LOCATION_REPORT}},
		"http://callback.example.test", "corr-id", "nf-id",
	)
	mode := models.NewAmfEventMode(models.AMFEVENTTRIGGER_CONTINUOUS)
	mode.SetExpiry(original)
	shared.SetOptions(*mode)

	// Both UEs hold distinct wrappers that point at the one shared subscription, exactly as
	// CreateAMFEventSubscriptionProcedure wires an any-UE subscription.
	newUe := func() *AmfUe {
		ue := &AmfUe{EventSubscriptionsInfo: make(map[string]*AmfUeEventSubscription)}
		ue.EventSubscriptionsInfo[id] = &AmfUeEventSubscription{EventSubscription: shared}
		return ue
	}
	ue1, ue2 := newUe(), newUe()

	updated := time.Now().Add(24 * time.Hour)
	ue1.SetEventSubscriptionExpiry(id, updated)

	opts1, _ := ue1.EventSubscriptionsInfo[id].EventSubscription.GetOptionsOk()
	if got1 := opts1.GetExpiry(); !got1.Equal(updated) {
		t.Errorf("updated UE expiry = %v, want %v", got1, updated)
	}

	opts2, _ := ue2.EventSubscriptionsInfo[id].EventSubscription.GetOptionsOk()
	if got2 := opts2.GetExpiry(); !got2.Equal(original) {
		t.Errorf("other UE expiry = %v, want it untouched at %v: the shared subscription was mutated", got2, original)
	}

	if ue1.EventSubscriptionsInfo[id].EventSubscription == ue2.EventSubscriptionsInfo[id].EventSubscription {
		t.Error("the updated UE still shares the subscription object with the other UE")
	}
}
