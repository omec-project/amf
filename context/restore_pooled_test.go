// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	gojson "github.com/goccy/go-json"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/mongoapi"
)

// liveUeWithItsDocument registers a UE on a RAN of its own, as the registration path leaves
// it -- in UePool, its RanUe in RanUePool and the RAN's list -- and returns the document
// StoreContextInDB would write for it.
func liveUeWithItsDocument(t *testing.T, supi string, ranUeNgapID int64) (*AmfRan, *AmfUe, *RanUe, map[string]any) {
	t.Helper()

	self := AMF_Self()
	gnbID := "208:93:" + supi
	ran := self.NewAmfRanId(gnbID)
	ran.AnType = models.ACCESSTYPE__3_GPP_ACCESS
	t.Cleanup(func() { self.AmfRanPool.Delete(gnbID) })

	// Built rather than NewAmfUe'd: that allocates a GUTI from a served GUAMI, and this
	// package's tests configure none.
	ue := &AmfUe{}
	ue.init()
	self.AddAmfUeToUePool(ue, supi)
	// A strict decode refuses an empty ngKSI type, as for a UE stored before it authenticated.
	ue.NgKsi = models.NgKsi{Tsc: models.SCTYPE_NATIVE, Ksi: 0}
	ranUe, err := ran.NewRanUe(ranUeNgapID)
	if err != nil {
		t.Fatalf("NewRanUe: %v", err)
	}
	ue.AttachRanUe(ranUe)
	t.Cleanup(func() {
		self.UePool.Delete(supi)
		self.RanUePool.Delete(ranUe.AmfUeNgapId)
	})

	raw, err := gojson.Marshal(ue)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	doc := map[string]any{}
	if err = gojson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("to map: %v", err)
	}

	return ran, ue, ranUe, doc
}

// serveStoredDocument makes doc the only stored context, with the datastore enabled.
func serveStoredDocument(t *testing.T, doc map[string]any) {
	t.Helper()

	self := AMF_Self()
	originalClient, originalStore, originalDrsm := mongoapi.CommonDBClient, self.EnableDbStore, self.Drsm
	mongoapi.CommonDBClient, self.EnableDbStore, self.Drsm = oneDocDB{doc: doc}, true, releasingDrsm{}
	t.Cleanup(func() {
		mongoapi.CommonDBClient, self.EnableDbStore, self.Drsm = originalClient, originalStore, originalDrsm
	})
}

// A release removes the UE's RanUe first and the UE itself only afterwards, once the release
// has decided to delete it. In between, the UE is still in UePool, its stored context is still
// there, and a lookup by the RanUe's id misses RanUePool and the RAN's list and falls through to
// the datastore. That lookup used to restore the stored copy over the UE the AMF holds, and
// hand back a RanUe the release had just removed.
func TestALookupByIdDuringAReleaseDoesNotRestoreTheUe(t *testing.T) {
	t.Run("by AMF-UE-NGAP-ID", func(t *testing.T) {
		const supi = "imsi-208930100009921"
		self := AMF_Self()

		_, ue, ranUe, doc := liveUeWithItsDocument(t, supi, 921)
		serveStoredDocument(t, doc)
		id := ranUe.AmfUeNgapId
		if err := ranUe.Remove(); err != nil {
			t.Fatalf("removing the RanUe: %v", err)
		}

		if found := self.RanUeFindByAmfUeNgapID(id); found != nil {
			t.Errorf("a lookup by the removed RanUe's AMF-UE-NGAP-ID %d found a RanUe", id)
		}
		if pooled, _ := self.UePool.Load(supi); pooled != ue {
			t.Error("the lookup replaced the UE the AMF holds with its stored copy")
		}
		if _, pooled := self.RanUePool.Load(id); pooled {
			t.Errorf("the lookup put a RanUe back in RanUePool under %d", id)
		}
	})

	t.Run("by RAN-UE-NGAP-ID", func(t *testing.T) {
		const supi = "imsi-208930100009922"
		self := AMF_Self()

		ran, ue, ranUe, doc := liveUeWithItsDocument(t, supi, 922)
		serveStoredDocument(t, doc)
		if err := ranUe.Remove(); err != nil {
			t.Fatalf("removing the RanUe: %v", err)
		}

		if found := ran.RanUeFindByRanUeNgapID(922); found != nil {
			t.Error("a lookup by the removed RanUe's RAN-UE-NGAP-ID found a RanUe")
		}
		if pooled, _ := self.UePool.Load(supi); pooled != ue {
			t.Error("the lookup replaced the UE the AMF holds with its stored copy")
		}
		if ran.RanUeFindByRanUeNgapIDLocal(922) != nil {
			t.Error("the lookup put a RanUe back in the RAN's list")
		}
	})
}

// Two lookups for the same UE that both miss the pool both restore it. The later one used to
// publish its copy over the earlier one's, so the two callers held different contexts for one
// UE and whatever the first did to its copy was lost.
func TestTwoRestoresOfOneUeShareOneContext(t *testing.T) {
	const supi = "imsi-208930100009923"
	self := AMF_Self()

	fake := &deletingDB{
		doc:           storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}),
		deleteEntered: make(chan struct{}),
		releaseDelete: make(chan struct{}),
	}
	installDeletingDB(t, fake, supi)

	// The second lookup runs to completion while the first is reading.
	var second *AmfUe
	reads := 0
	fake.afterRead = func() {
		reads++
		if reads == 1 {
			second, _ = self.AmfUeFindBySupi(supi)
		}
	}

	first, found := self.AmfUeFindBySupi(supi)

	if !found || first == nil || second == nil {
		t.Fatalf("a restore did not find the UE: first %p, second %p", first, second)
	}
	if first != second {
		t.Error("two restores of one UE returned two different contexts")
	}
	if pooled, _ := self.UePool.Load(supi); pooled != second {
		t.Error("the later restore replaced the context the earlier one had published")
	}
}

// A lookup by an id the UE no longer has, after it has moved to a newer RanUe, finds no RanUe.
// The AMF already holds the UE, so the restore returns that UE in place of its stored copy;
// its RanUe then is the newer one, and handing that back for the old id would act on the
// UE's current association in answer to a message about its last one. RAN-UE-NGAP-IDs are
// per RAN, so the same number on another RAN names a different RanUe too.
func TestALookupByAnOldIdDoesNotFindTheUesNewerRanUe(t *testing.T) {
	// moveToNewerRanUe ends the UE's first RanUe and attaches a newer one on ran.
	moveToNewerRanUe := func(t *testing.T, ue *AmfUe, first *RanUe, ran *AmfRan, ranUeNgapID int64) *RanUe {
		t.Helper()

		if err := first.Remove(); err != nil {
			t.Fatalf("removing the first RanUe: %v", err)
		}
		newer, err := ran.NewRanUe(ranUeNgapID)
		if err != nil {
			t.Fatalf("NewRanUe: %v", err)
		}
		ue.AttachRanUe(newer)
		t.Cleanup(func() { AMF_Self().RanUePool.Delete(newer.AmfUeNgapId) })

		return newer
	}

	t.Run("by AMF-UE-NGAP-ID", func(t *testing.T) {
		const supi = "imsi-208930100009924"
		self := AMF_Self()

		ran, ue, first, doc := liveUeWithItsDocument(t, supi, 924)
		newer := moveToNewerRanUe(t, ue, first, ran, 925)
		serveStoredDocument(t, doc)

		if found := self.RanUeFindByAmfUeNgapID(first.AmfUeNgapId); found != nil {
			t.Errorf("a lookup by the old AMF-UE-NGAP-ID %d found a RanUe (AMF-UE-NGAP-ID %d)",
				first.AmfUeNgapId, found.AmfUeNgapId)
		}
		if ue.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS) != newer {
			t.Error("the lookup changed the UE's RanUe")
		}
	})

	t.Run("by RAN-UE-NGAP-ID on the same RAN", func(t *testing.T) {
		const supi = "imsi-208930100009926"

		ran, ue, first, doc := liveUeWithItsDocument(t, supi, 926)
		newer := moveToNewerRanUe(t, ue, first, ran, 927)
		serveStoredDocument(t, doc)

		if found := ran.RanUeFindByRanUeNgapID(926); found != nil {
			t.Errorf("a lookup by the old RAN-UE-NGAP-ID found a RanUe (RAN-UE-NGAP-ID %d)", found.RanUeNgapId)
		}
		if ue.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS) != newer || newer.Ran != ran {
			t.Error("the lookup changed the UE's RanUe")
		}
	})

	t.Run("by RAN-UE-NGAP-ID now used on another RAN", func(t *testing.T) {
		const supi = "imsi-208930100009928"
		self := AMF_Self()

		ran, ue, first, doc := liveUeWithItsDocument(t, supi, 928)
		other := self.NewAmfRanId("208:93:other-" + supi)
		other.AnType = models.ACCESSTYPE__3_GPP_ACCESS
		t.Cleanup(func() { self.AmfRanPool.Delete("208:93:other-" + supi) })
		newer := moveToNewerRanUe(t, ue, first, other, 928)
		serveStoredDocument(t, doc)

		if found := ran.RanUeFindByRanUeNgapID(928); found != nil {
			t.Error("a lookup on one RAN found the UE's RanUe on another RAN with the same RAN-UE-NGAP-ID")
		}
		if newer.Ran != other {
			t.Error("the lookup moved the UE's RanUe onto the RAN it asked")
		}
		if ran.RanUeFindByRanUeNgapIDLocal(928) != nil {
			t.Error("the lookup put a RanUe in the asking RAN's list")
		}
	})
}

// After a restart, the first message about a UE restores it, by whichever id that message
// carries, and later ones find it. A restore by RAN-UE-NGAP-ID returns the stored RanUe and
// lists it on the RAN; and a lookup by RAN-UE-NGAP-ID after a restore by AMF-UE-NGAP-ID finds
// the RanUe that restore published. That second lookup used to restore the UE again, and
// return a RanUe of another copy.
func TestALookupAfterARestartFindsTheRestoredRanUe(t *testing.T) {
	// restarted leaves the UE known only by its stored context, with its gNB still connected.
	restarted := func(t *testing.T, supi string, ranUeNgapID int64) (*AmfRan, *RanUe, map[string]any) {
		t.Helper()

		ran, ue, ranUe, doc := liveUeWithItsDocument(t, supi, ranUeNgapID)
		ue.Remove()
		serveStoredDocument(t, doc)

		return ran, ranUe, doc
	}

	t.Run("restored by RAN-UE-NGAP-ID", func(t *testing.T) {
		const supi = "imsi-208930100009929"

		ran, stored, _ := restarted(t, supi, 929)

		found := ran.RanUeFindByRanUeNgapID(929)
		if found == nil {
			t.Fatal("a lookup by RAN-UE-NGAP-ID did not restore the UE")
		}
		if found.AmfUeNgapId != stored.AmfUeNgapId || found.Ran != ran {
			t.Errorf("restored RanUe has AMF-UE-NGAP-ID %d on %p, stored %d on %p",
				found.AmfUeNgapId, found.Ran, stored.AmfUeNgapId, ran)
		}
		if ran.RanUeFindByRanUeNgapIDLocal(929) != found {
			t.Error("the restored RanUe is not in the RAN's list")
		}
	})

	t.Run("restored by AMF-UE-NGAP-ID, then found by RAN-UE-NGAP-ID", func(t *testing.T) {
		const supi = "imsi-208930100009930"
		self := AMF_Self()

		ran, stored, _ := restarted(t, supi, 930)

		first := self.RanUeFindByAmfUeNgapID(stored.AmfUeNgapId)
		if first == nil {
			t.Fatal("a lookup by AMF-UE-NGAP-ID did not restore the UE")
		}
		second := ran.RanUeFindByRanUeNgapID(930)
		if second != first {
			t.Error("a lookup by RAN-UE-NGAP-ID found another RanUe than the restore published")
		}
		if ran.RanUeFindByRanUeNgapIDLocal(930) != first {
			t.Error("the restored RanUe is not in the RAN's list")
		}
	})
}
