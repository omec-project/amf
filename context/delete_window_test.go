// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"bytes"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/drsm"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// releasingDrsm accepts every release; Remove releases the UE's TMSI through it once the
// datastore is enabled.
type releasingDrsm struct{ drsm.DrsmInterface }

func (releasingDrsm) ReleaseInt32ID(int32) error { return nil }

// deletingDB serves one stored document until a delete of it lands, and can hold that
// delete in flight: it signals on deleteEntered and waits on releaseDelete. If afterRead
// is set, a read runs it once it has taken the document and before returning it.
type deletingDB struct {
	mongoapi.DBInterface
	mu            sync.Mutex
	doc           map[string]any
	deleteEntered chan struct{}
	releaseDelete chan struct{}
	afterRead     func()
}

func (f *deletingDB) RestfulAPIGetOne(string, bson.M) (map[string]any, error) {
	out, err := f.read()
	if f.afterRead != nil {
		f.afterRead()
	}
	return out, err
}

func (f *deletingDB) read() (map[string]any, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.doc == nil {
		return nil, nil
	}
	// A fresh copy each time, as a real read returns: DbFetch edits what it is given.
	raw, err := bson.Marshal(f.doc)
	if err != nil {
		return nil, err
	}
	decoder := bson.NewDecoder(bson.NewDocumentReader(bytes.NewReader(raw)))
	decoder.DefaultDocumentMap()
	var out map[string]any
	return out, decoder.Decode(&out)
}

func (f *deletingDB) RestfulAPIDeleteOne(string, bson.M) error {
	close(f.deleteEntered)
	<-f.releaseDelete
	f.mu.Lock()
	f.doc = nil
	f.mu.Unlock()
	return nil
}

// A UE's release removes it from the pool and deletes its stored context. A lookup for
// that UE in between -- a registration arriving on another association -- used to miss
// the pool, fall through to the datastore, and restore the context about to be deleted.
func TestAUeBeingReleasedIsNotRestoredFromItsStoredContext(t *testing.T) {
	const supi = "imsi-208930100009801"
	self := AMF_Self()
	fake := &deletingDB{
		doc:           storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}),
		deleteEntered: make(chan struct{}),
		releaseDelete: make(chan struct{}),
	}
	w := newDBWriter(1, 4)
	originalWriter, originalClient := amfUeWriter, mongoapi.CommonDBClient
	originalStore, originalDrsm := self.EnableDbStore, self.Drsm
	amfUeWriter, mongoapi.CommonDBClient = w, fake
	self.EnableDbStore, self.Drsm = true, releasingDrsm{}
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(fake.releaseDelete) })
		amfUeWriter, mongoapi.CommonDBClient = originalWriter, originalClient
		self.EnableDbStore, self.Drsm = originalStore, originalDrsm
		self.UePool.Delete(supi)
		for _, queue := range w.queues {
			close(queue)
		}
	})
	w.start()

	ue := storableUe(supi)
	self.UePool.Store(supi, ue)

	released := make(chan struct{})
	go func() {
		RemoveUeAndDeleteItsContext(ue)
		close(released)
	}()
	<-fake.deleteEntered

	restored, found := self.AmfUeFindBySupi(supi)
	releaseOnce.Do(func() { close(fake.releaseDelete) })
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("the release never finished")
	}

	if found {
		t.Fatalf("a lookup during the release restored the context being deleted (%p)", restored)
	}
	if _, pooled := self.UePool.Load(supi); pooled {
		t.Error("the released UE is back in the pool")
	}
}

// The same lookup, but its read takes the document before the delete lands and is slow
// to return it: by the time it does, the release may have finished and cleared its mark.
// A check of the mark alone would then find none and restore the context the delete has
// just removed.
func TestALookupWhoseReadPrecedesTheDeleteDoesNotRestoreIt(t *testing.T) {
	const supi = "imsi-208930100009802"
	self := AMF_Self()
	fake := &deletingDB{
		doc:           storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}),
		deleteEntered: make(chan struct{}),
		releaseDelete: make(chan struct{}),
	}
	w := newDBWriter(1, 4)
	originalWriter, originalClient := amfUeWriter, mongoapi.CommonDBClient
	originalStore, originalDrsm := self.EnableDbStore, self.Drsm
	amfUeWriter, mongoapi.CommonDBClient = w, fake
	self.EnableDbStore, self.Drsm = true, releasingDrsm{}
	var releaseOnce sync.Once
	t.Cleanup(func() {
		releaseOnce.Do(func() { close(fake.releaseDelete) })
		amfUeWriter, mongoapi.CommonDBClient = originalWriter, originalClient
		self.EnableDbStore, self.Drsm = originalStore, originalDrsm
		self.UePool.Delete(supi)
		for _, queue := range w.queues {
			close(queue)
		}
	})
	w.start()

	ue := storableUe(supi)
	self.UePool.Store(supi, ue)

	released := make(chan struct{})
	go func() {
		RemoveUeAndDeleteItsContext(ue)
		close(released)
	}()
	<-fake.deleteEntered

	// The first read hands its document back only once the release has returned, so
	// the lookup checks the mark after it has been cleared.
	readTaken := make(chan struct{})
	var firstRead sync.Once
	fake.afterRead = func() {
		firstRead.Do(func() {
			close(readTaken)
			<-released
		})
	}
	type lookup struct {
		ue    *AmfUe
		found bool
	}
	looked := make(chan lookup, 1)
	go func() {
		restored, found := self.AmfUeFindBySupi(supi)
		looked <- lookup{restored, found}
	}()
	<-readTaken
	releaseOnce.Do(func() { close(fake.releaseDelete) })

	var got lookup
	select {
	case got = <-looked:
	case <-time.After(5 * time.Second):
		t.Fatal("the lookup never finished")
	}
	select {
	case <-released:
	case <-time.After(5 * time.Second):
		t.Fatal("the release never finished")
	}

	if got.found {
		t.Fatalf("a lookup whose read preceded the delete restored the deleted context (%p)", got.ue)
	}
	if _, pooled := self.UePool.Load(supi); pooled {
		t.Error("the released UE is back in the pool")
	}
}
