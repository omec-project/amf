// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"testing"

	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// countingDocDB serves one stored document to every read and counts the reads.
type countingDocDB struct {
	mongoapi.DBInterface
	doc   map[string]any
	reads int
}

func (s *countingDocDB) RestfulAPIGetOne(string, bson.M) (map[string]any, error) {
	s.reads++
	return s.doc, nil
}

// withStoredDocument serves doc as the only stored context, with the datastore enabled,
// and cleans up whatever a restore publishes for supi.
func withStoredDocument(t *testing.T, supi string, doc map[string]any) *countingDocDB {
	t.Helper()

	self := AMF_Self()
	fake := &countingDocDB{doc: doc}
	originalClient, originalStore := mongoapi.CommonDBClient, self.EnableDbStore
	mongoapi.CommonDBClient, self.EnableDbStore = fake, true
	t.Cleanup(func() {
		mongoapi.CommonDBClient, self.EnableDbStore = originalClient, originalStore
		self.UePool.Delete(supi)
		self.RanUePool.Delete(int64(0))
	})

	return fake
}

// Every stored context without a RAN association carries amfUeNgapId 0, and the AMF
// never allocates 0 to a live UE, so 0 names no UE. A lookup by it used to fall through
// to the datastore and match whichever detached context came back first -- which the
// NGAP handlers then attached to the association the message arrived on.
func TestAnAmfUeNgapIdOfZeroFindsNoUe(t *testing.T) {
	const supi = "imsi-208930100009701"
	fake := withStoredDocument(t, supi,
		storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}))

	if ranUe := AMF_Self().RanUeFindByAmfUeNgapID(0); ranUe != nil {
		t.Fatalf("a lookup by AMF-UE-NGAP-ID 0 found a UE (%s); 0 names none", ranUe.AmfUe.GetSupi())
	}
	if fake.reads != 0 {
		t.Errorf("the datastore was read %d time(s) for AMF-UE-NGAP-ID 0", fake.reads)
	}
}

// Restoring a detached context by SUPI or GUTI must not make it findable by AMF-UE-NGAP-ID:
// its RanUe carries 0, and every detached context restored this way would claim that key.
func TestARestoredDetachedContextIsNotPublishedUnderAmfUeNgapIdZero(t *testing.T) {
	const supi = "imsi-208930100009702"
	withStoredDocument(t, supi,
		storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}))

	restored := DbFetch(AmfUeDataColl, bson.M{supiField: supi})
	if restored == nil {
		t.Fatal("DbFetch could not restore the stored context")
		return
	}
	if _, pooled := AMF_Self().UePool.Load(supi); !pooled {
		t.Error("the restored UE was not published by SUPI")
	}
	if _, pooled := AMF_Self().RanUePool.Load(int64(0)); pooled {
		t.Error("the restored detached context was published under AMF-UE-NGAP-ID 0")
	}
}
