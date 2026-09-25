// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"slices"
	"testing"

	"github.com/bytedance/sonic"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// manyDocsDB serves a fixed set of stored documents to RestfulAPIGetMany and nothing
// else; every other DBInterface method is promoted from a nil interface.
type manyDocsDB struct {
	mongoapi.DBInterface
	docs []map[string]any
}

func (s manyDocsDB) RestfulAPIGetMany(string, bson.M) ([]map[string]any, error) {
	return s.docs, nil
}

// storedDocument marshals ue the way StoreContextInDB does and returns the map a read
// hands back.
func storedDocument(t *testing.T, ue *AmfUe) map[string]any {
	t.Helper()

	raw, err := sonic.Marshal(ue)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	doc := map[string]any{}
	if err := sonic.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("to map: %v", err)
	}

	return doc
}

// Listing the stored contexts stopped at the first one it could not decode and returned
// nothing at all, so one bad record hid every good one. It also had none of DbFetch's
// retry, so a record DbFetch can read -- one with an empty enum value -- counted as bad.
func TestDbFetchAllEntriesListsEveryReadableContext(t *testing.T) {
	complete := &AmfUe{}
	complete.init()
	complete.SetSupi("imsi-208930100009601")
	complete.NgKsi = models.NgKsi{Tsc: models.SCTYPE_NATIVE, Ksi: 0}

	// Stored before authentication finished: ngKsi.tsc is empty, which the strict
	// decode refuses and the retry recovers.
	early := &AmfUe{}
	early.init()
	early.SetSupi("imsi-208930100009602")

	unreadable := map[string]any{"supi": 208930100009603}

	original := mongoapi.CommonDBClient
	mongoapi.CommonDBClient = manyDocsDB{docs: []map[string]any{
		storedDocument(t, complete), unreadable, storedDocument(t, early),
	}}
	t.Cleanup(func() { mongoapi.CommonDBClient = original })

	var supis []string
	for _, ue := range DbFetchAllEntries() {
		supis = append(supis, ue.GetSupi())
	}

	want := []string{"imsi-208930100009601", "imsi-208930100009602"}
	if !slices.Equal(supis, want) {
		t.Fatalf("listed %v, want %v: every readable context, with only the unreadable one left out",
			supis, want)
	}
}
