// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"bytes"
	"slices"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// mongoShaped returns doc the way the MongoDB driver hands a document back to
// RestfulAPIGetOne. The client decodes with DefaultDocumentMap, so documents are
// map[string]any -- but arrays are bson.A, a named type that a []any case does not
// match. A fixture built by decoding JSON has []any arrays instead, which is how the
// recovery path's own tests missed this.
func mongoShaped(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()

	raw, err := bson.Marshal(doc)
	if err != nil {
		t.Fatalf("bson.Marshal: %v", err)
	}
	decoder := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(raw)))
	decoder.DefaultDocumentMap()
	var out map[string]any
	if err := decoder.Decode(&out); err != nil {
		t.Fatalf("decode as the driver does: %v", err)
	}

	return out
}

// storedUeWithTriggers stores a UE whose AM policy association carries the given
// triggers, and returns the document as DbFetch receives it.
func storedUeWithTriggers(t *testing.T, supi string, triggers []models.RequestTrigger) map[string]any {
	t.Helper()

	ue := &AmfUe{}
	ue.init()
	ue.SetSupi(supi)
	// A UE stored before authentication finished has an empty ngKSI type of security
	// context, which the strict decode also refuses; a real one keeps this test about
	// the triggers alone.
	ue.NgKsi = models.NgKsi{Tsc: models.SCTYPE_NATIVE, Ksi: 0}
	ue.AmPolicyAssociation = &models.PolicyAssociation{Triggers: triggers}

	raw, err := sonic.Marshal(ue)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	doc := map[string]any{}
	if err := sonic.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("to map: %v", err)
	}

	return mongoShaped(t, doc)
}

func restoreFrom(t *testing.T, supi string, doc map[string]any) *AmfUe {
	t.Helper()

	original := mongoapi.CommonDBClient
	mongoapi.CommonDBClient = oneDocDB{doc: doc}

	restored := DbFetch(AmfUeDataColl, bson.M{"supi": supi})
	// DbFetch publishes the restored UE into UePool and its RanUe into RanUePool. The
	// fixture has no RAN, so that RanUe carries AmfUeNgapId 0 -- still an entry to remove.
	t.Cleanup(func() {
		mongoapi.CommonDBClient = original
		AMF_Self().UePool.Delete(supi)
		if restored != nil {
			if ranUe := restored.RanUe[models.ACCESSTYPE__3_GPP_ACCESS]; ranUe != nil {
				AMF_Self().RanUePool.Delete(ranUe.AmfUeNgapId)
			}
		}
	})

	return restored
}

// A stored context holding an empty value in an enum array -- here the AM policy
// association's triggers -- is refused by the strict decode. The retry that drops empty
// enum values could not recover it: arrays arrive as bson.A, which it never entered,
// and even as []any it could not remove an element that is itself the empty value.
func TestDbFetchRecoversAnEmptyValueInAStoredEnumArray(t *testing.T) {
	const supi = "imsi-208930100009501"
	doc := storedUeWithTriggers(t, supi, []models.RequestTrigger{"", models.REQUESTTRIGGER_LOC_CH})

	restored := restoreFrom(t, supi, doc)

	if restored == nil {
		t.Fatal("DbFetch could not read a stored context whose only fault is an empty trigger")
		return
	}
	if restored.AmPolicyAssociation == nil {
		t.Fatal("the restored context lost its AM policy association")
		return
	}
	want := []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}
	if got := restored.AmPolicyAssociation.Triggers; !slices.Equal(got, want) {
		t.Fatalf("triggers = %v, want %v: only the empty value may be dropped", got, want)
	}
}

// The control for the test above: the same stored context without the empty trigger
// reads back on the first, strict decode.
func TestDbFetchReadsAStoredEnumArrayWithNoEmptyValue(t *testing.T) {
	const supi = "imsi-208930100009502"
	doc := storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH})

	if restored := restoreFrom(t, supi, doc); restored == nil {
		t.Fatal("DbFetch could not read a well-formed stored context")
	}
}

// dropEmptyEnumValues must reach empty values inside arrays as the driver returns them:
// an empty element of an enum array, and an empty field of an object held in an array.
// Everything else stays.
func TestDropEmptyEnumValuesReachesIntoStoredArrays(t *testing.T) {
	doc := mongoShaped(t, map[string]any{
		"enums":   []any{"", "NR", ""},
		"objects": []any{map[string]any{"class": ""}, map[string]any{"class": "SM"}},
		"numbers": []any{0, 1},
	})

	dropEmptyEnumValues(doc)

	enums, _ := doc["enums"].(bson.A)
	if !slices.Equal([]any(enums), []any{"NR"}) {
		t.Errorf("enums = %#v, want only the non-empty value", doc["enums"])
	}
	objects, _ := doc["objects"].(bson.A)
	if len(objects) != 2 {
		t.Fatalf("objects = %#v, want both objects kept", doc["objects"])
	}
	if first, _ := objects[0].(map[string]any); first == nil || first["class"] != nil {
		t.Errorf("objects[0] = %#v, want its empty class dropped", objects[0])
	}
	if second, _ := objects[1].(map[string]any); second == nil || second["class"] != "SM" {
		t.Errorf("objects[1] = %#v, want its class kept", objects[1])
	}
	if numbers, _ := doc["numbers"].(bson.A); len(numbers) != 2 {
		t.Errorf("numbers = %#v, want both kept", doc["numbers"])
	}
}
