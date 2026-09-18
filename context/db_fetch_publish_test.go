// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/bytedance/sonic"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// oneDocDB serves a single stored document and nothing else. DBInterface is embedded rather
// than implemented: the other twenty-one methods are promoted from a nil interface, so if
// DbFetch ever reaches for one this test panics instead of quietly passing.
type oneDocDB struct {
	mongoapi.DBInterface
	doc map[string]any
}

func (s oneDocDB) RestfulAPIGetOne(string, bson.M) (map[string]any, error) {
	return s.doc, nil
}

// storedUeDocument round-trips a UE through MarshalJSON into the map shape DbFetch reads, so
// the fixture is whatever StoreContextInDB would actually have written.
func storedUeDocument(t *testing.T, supi string) map[string]any {
	t.Helper()

	ran := AMF_Self().NewAmfRanId("208:93:storeddoc")
	t.Cleanup(func() { AMF_Self().AmfRanPool.Delete("208:93:storeddoc") })

	ue := &AmfUe{}
	ue.init()
	ue.SetSupi(supi)
	ue.NgKsi = models.NgKsi{Tsc: models.SCTYPE_NATIVE, Ksi: 0}

	ranUe, err := ran.NewRanUe(21)
	if err != nil {
		t.Fatalf("NewRanUe: %v", err)
	}
	ue.AttachRanUe(ranUe)
	t.Cleanup(func() { AMF_Self().RanUePool.Delete(ranUe.AmfUeNgapId) })

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

// A restored UE is published into UePool and RanUePool, and every reader that finds it there
// expects a usable context: loggers, instance identity, no stale event channel. DbFetch used
// to publish first and fill those in afterwards, so a lookup landing in between got a
// half-built context.
//
// Be clear about what this does and does not do. It passes under the old order as well --
// checked, not assumed -- because by the time DbFetch returns the context is complete either
// way. It is not a regression test for the ordering: that window has no hook to drive it from
// a unit test, and a probabilistic one would be worth less than the argument in the code,
// which is that nothing between the two positions reads either pool.
//
// What it does pin is the post-condition a reader depends on, expressed through the pool
// rather than the return value, and -- via the embedded nil DBInterface -- that DbFetch needs
// nothing from the datastore but the one read. Both are what make publishing last safe, and
// both would catch a later change that reintroduced a pool read between building and
// publishing.
func TestDbFetchPublishesAFullyInitialisedContext(t *testing.T) {
	const supi = "imsi-208930100009101"

	t.Setenv("HOSTNAME", "amf-under-test")
	t.Setenv("POD_IP", "10.0.0.9")

	doc := storedUeDocument(t, supi)

	original := mongoapi.CommonDBClient
	mongoapi.CommonDBClient = oneDocDB{doc: doc}
	t.Cleanup(func() { mongoapi.CommonDBClient = original })

	restored := DbFetch(AmfUeDataColl, bson.M{"supi": supi})
	if restored == nil {
		t.Fatal("DbFetch returned nil for a document it was handed")
		return
	}
	t.Cleanup(func() {
		AMF_Self().UePool.Delete(supi)
		if ranUe := restored.RanUe[models.ACCESSTYPE__3_GPP_ACCESS]; ranUe != nil {
			AMF_Self().RanUePool.Delete(ranUe.AmfUeNgapId)
		}
	})

	pooled, ok := AMF_Self().AmfUeFindBySupiLocal(supi)
	if !ok {
		t.Fatal("restored UE was never published into UePool")
		return
	}
	if pooled != restored {
		t.Fatal("UePool holds a different UE than DbFetch returned")
	}

	// Read from the pooled pointer, not the returned one: the pool is how every other
	// goroutine reaches this UE, and it is what the publication order is about.
	if pooled.NASLog == nil || pooled.GmmLog == nil || pooled.TxLog == nil || pooled.ProducerLog == nil {
		t.Fatal("UE published into the pool has no per-UE loggers")
	}
	if pooled.AmfInstanceName != "amf-under-test" || pooled.AmfInstanceIp != "10.0.0.9" {
		t.Fatalf("UE published into the pool has no instance identity: name=%q ip=%q",
			pooled.AmfInstanceName, pooled.AmfInstanceIp)
	}
	if pooled.EventChannel != nil {
		t.Fatal("restored UE kept an event channel the restore is supposed to clear")
	}

	ranUe := pooled.RanUe[models.ACCESSTYPE__3_GPP_ACCESS]
	if ranUe == nil {
		t.Fatal("restored UE has no 3GPP RanUe")
		return
	}
	if _, ok := AMF_Self().RanUePool.Load(ranUe.AmfUeNgapId); !ok {
		t.Fatal("restored RanUe was never published into RanUePool")
	}
}
