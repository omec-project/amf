// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"slices"
	"testing"

	gojson "github.com/goccy/go-json"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/drsm"
	"github.com/omec-project/util/mongoapi"
)

// recordingDrsm records the ids released to it.
type recordingDrsm struct {
	drsm.DrsmInterface
	released []int32
}

func (d *recordingDrsm) ReleaseInt32ID(id int32) error {
	d.released = append(d.released, id)
	return nil
}

// restoreBySupi serves doc as the only stored context, with the datastore enabled and d as
// the DRSM, and restores the UE by SUPI.
func restoreBySupi(t *testing.T, supi string, doc map[string]any, d drsm.DrsmInterface) *AmfUe {
	t.Helper()

	self := AMF_Self()
	originalClient, originalStore, originalDrsm := mongoapi.CommonDBClient, self.EnableDbStore, self.Drsm
	mongoapi.CommonDBClient, self.EnableDbStore, self.Drsm = oneDocDB{doc: doc}, true, d
	t.Cleanup(func() {
		mongoapi.CommonDBClient, self.EnableDbStore, self.Drsm = originalClient, originalStore, originalDrsm
		self.UePool.Delete(supi)
	})

	ue, found := self.AmfUeFindBySupi(supi)
	if !found || ue == nil {
		t.Fatal("the stored context was not restored")
	}

	return ue
}

// The custom fields of a stored context describe its 3GPP RanUe alone; MarshalJSON takes them
// from it. The restore applied them to a RanUe for every access type in the UE's state, and
// the state always holds both, so a UE stored on 3GPP access came back with a non-3GPP RanUe
// too, carrying the 3GPP ids and the 3GPP RAN.
func TestARestoredUeHasARanUeOnlyForTheAccessItWasStoredOn(t *testing.T) {
	const supi = "imsi-208930100009911"

	doc := storedUeDocument(t, supi)
	stored := doc["customFieldsAmfUe"].(map[string]any)
	ue := restoreBySupi(t, supi, doc, &recordingDrsm{})

	if ranUe := ue.GetRanUe(models.ACCESSTYPE_NON_3_GPP_ACCESS); ranUe != nil {
		t.Errorf("a UE stored on 3GPP access was restored with a non-3GPP RanUe (AMF-UE-NGAP-ID %d)",
			ranUe.AmfUeNgapId)
	}

	ranUe := ue.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS)
	if ranUe == nil {
		t.Fatal("the UE was restored without its 3GPP RanUe")
	}
	if float64(ranUe.AmfUeNgapId) != stored["amfUeNgapId"] || float64(ranUe.RanUeNgapId) != stored["ranUeNgapId"] {
		t.Errorf("3GPP RanUe restored with AMF-UE-NGAP-ID %d and RAN-UE-NGAP-ID %d, stored %v and %v",
			ranUe.AmfUeNgapId, ranUe.RanUeNgapId, stored["amfUeNgapId"], stored["ranUeNgapId"])
	}
	if ran, _ := AMF_Self().AmfRanFindByGnbId("208:93:storeddoc"); ranUe.Ran != ran {
		t.Error("the 3GPP RanUe was not attached to the connected gNB it was stored on")
	}
}

// A UE restored while its gNB is connected gives its AMF-UE-NGAP-ID back once when it is
// removed. The non-3GPP RanUe the restore created carried the same id and the same RAN, so
// Remove released the id twice, the second time possibly from under a UE that had been
// allocated it in between.
func TestRemovingARestoredUeReleasesItsAmfUeNgapIdOnce(t *testing.T) {
	const supi = "imsi-208930100009912"

	recorder := &recordingDrsm{}
	ue := restoreBySupi(t, supi, storedUeDocument(t, supi), recorder)
	id := int32(ue.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS).AmfUeNgapId)

	ue.Remove()

	released := 0
	for _, r := range recorder.released {
		if r == id {
			released++
		}
	}
	if released != 1 {
		t.Errorf("AMF-UE-NGAP-ID %d was released %d times, want once", id, released)
	}
}

// copyOf returns a deep copy of a document value.
func copyOf(t *testing.T, v any) any {
	t.Helper()

	raw, err := gojson.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var out any
	if err = gojson.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	return out
}

// An AMF that predates this change invented a non-3GPP RanUe at every restore and stored it
// with the context the next time the context was stored. Those documents are still in the
// datastore, so the restore must not take the invented RanUe back from them. These model the
// shapes they take. Stored again while the UE is connected, the invented RanUe has the 3GPP
// RanUe's ids and RAN. Stored again after an N2 release, which removes only the 3GPP RanUe, it
// still holds ids the custom fields no longer name, and the 3GPP RAN -- or none, if the gNB
// was not connected when the older AMF restored the UE.
func TestARestoreDropsTheNon3gppRanUeAnOlderRestoreStored(t *testing.T) {
	t.Run("stored while connected", func(t *testing.T) {
		const supi = "imsi-208930100009913"

		doc := storedUeDocument(t, supi)
		ranUes := doc["ranUe"].(map[string]any)
		ranUes[string(models.ACCESSTYPE_NON_3_GPP_ACCESS)] = copyOf(t, ranUes[string(models.ACCESSTYPE__3_GPP_ACCESS)])

		recorder := &recordingDrsm{}
		ue := restoreBySupi(t, supi, doc, recorder)
		if ranUe := ue.GetRanUe(models.ACCESSTYPE_NON_3_GPP_ACCESS); ranUe != nil {
			t.Fatalf("the restore took back the invented non-3GPP RanUe (AMF-UE-NGAP-ID %d)", ranUe.AmfUeNgapId)
		}
		id := int32(ue.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS).AmfUeNgapId)

		ue.Remove()

		if released := slices.Index(recorder.released, id); released < 0 ||
			slices.Index(recorder.released[released+1:], id) >= 0 {
			t.Errorf("AMF-UE-NGAP-ID %d was not released exactly once: released %v", id, recorder.released)
		}
	})

	afterN2Release := func(t *testing.T, supi string, withRan bool) {
		doc := storedUeDocument(t, supi)
		ranUes := doc["ranUe"].(map[string]any)
		invented := copyOf(t, ranUes[string(models.ACCESSTYPE__3_GPP_ACCESS)]).(map[string]any)
		if !withRan {
			invented["Ran"] = nil
		}
		doc["ranUe"] = map[string]any{string(models.ACCESSTYPE_NON_3_GPP_ACCESS): invented}
		custom := doc["customFieldsAmfUe"].(map[string]any)
		stale := int32(custom["amfUeNgapId"].(float64))
		custom["amfUeNgapId"], custom["ranUeNgapId"], custom["ranId"] = 0, 0, ""

		recorder := &recordingDrsm{}
		ue := restoreBySupi(t, supi, doc, recorder)
		if ranUe := ue.GetRanUe(models.ACCESSTYPE_NON_3_GPP_ACCESS); ranUe != nil {
			t.Fatalf("the restore took back the invented non-3GPP RanUe (AMF-UE-NGAP-ID %d)", ranUe.AmfUeNgapId)
		}

		ue.Remove()

		if slices.Contains(recorder.released, stale) {
			t.Errorf("removing the UE released AMF-UE-NGAP-ID %d, which its context no longer names", stale)
		}
	}

	t.Run("stored again after an N2 release", func(t *testing.T) {
		afterN2Release(t, "imsi-208930100009914", true)
	})

	t.Run("stored again after an N2 release, restored without its gNB", func(t *testing.T) {
		afterN2Release(t, "imsi-208930100009917", false)
	})
}

// A UE that really was on non-3GPP access keeps that RanUe through a restore, with its own
// ids. The restore used to give every RanUe the custom fields' ids, which describe the 3GPP
// RanUe alone and are zero for a UE with none.
func TestARestoredUeKeepsAGenuineNon3gppRanUe(t *testing.T) {
	const supi = "imsi-208930100009915"
	self := AMF_Self()

	n3iwf := self.NewAmfRanId("n3iwf:restorekeep")
	t.Cleanup(func() { self.AmfRanPool.Delete("n3iwf:restorekeep") })
	n3iwf.AnType = models.ACCESSTYPE_NON_3_GPP_ACCESS

	stored := &AmfUe{}
	stored.init()
	stored.SetSupi(supi)
	stored.NgKsi = models.NgKsi{Tsc: models.SCTYPE_NATIVE, Ksi: 0}
	n3gppRanUe, err := n3iwf.NewRanUe(8)
	if err != nil {
		t.Fatalf("NewRanUe: %v", err)
	}
	stored.AttachRanUe(n3gppRanUe)
	t.Cleanup(func() { self.RanUePool.Delete(n3gppRanUe.AmfUeNgapId) })

	raw, err := gojson.Marshal(stored)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	doc := map[string]any{}
	if err = gojson.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("to map: %v", err)
	}

	ue := restoreBySupi(t, supi, doc, &recordingDrsm{})

	ranUe := ue.GetRanUe(models.ACCESSTYPE_NON_3_GPP_ACCESS)
	if ranUe == nil {
		t.Fatal("the UE's non-3GPP RanUe was not restored")
	}
	if ranUe.AmfUeNgapId != n3gppRanUe.AmfUeNgapId || ranUe.RanUeNgapId != 8 {
		t.Errorf("non-3GPP RanUe restored with AMF-UE-NGAP-ID %d and RAN-UE-NGAP-ID %d, stored %d and 8",
			ranUe.AmfUeNgapId, ranUe.RanUeNgapId, n3gppRanUe.AmfUeNgapId)
	}
	if ranUe.Log == nil {
		t.Error("the restored non-3GPP RanUe has no logger")
	}
}

// A null RanUe in the document restores as no entry at all: CmConnect reads the key's presence
// as a connection.
func TestARestoreSkipsANullRanUe(t *testing.T) {
	const supi = "imsi-208930100009916"

	doc := storedUeDocument(t, supi)
	doc["ranUe"].(map[string]any)[string(models.ACCESSTYPE_NON_3_GPP_ACCESS)] = nil

	ue := restoreBySupi(t, supi, doc, &recordingDrsm{})

	if ue.CmConnect(models.ACCESSTYPE_NON_3_GPP_ACCESS) {
		t.Error("a null non-3GPP RanUe left the UE reported as connected on non-3GPP access")
	}
}
