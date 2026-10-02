// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/omec-project/amf/logger"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/drsm"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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

// installDeletingDB swaps in fake and a datastore writer for the length of the test.
func installDeletingDB(t *testing.T, fake *deletingDB, supi string) {
	t.Helper()

	self := AMF_Self()
	w := newDBWriter(1, 4)
	originalWriter, originalClient := amfUeWriter, mongoapi.CommonDBClient
	originalStore, originalDrsm := self.EnableDbStore, self.Drsm
	amfUeWriter, mongoapi.CommonDBClient = w, fake
	self.EnableDbStore, self.Drsm = true, releasingDrsm{}
	t.Cleanup(func() {
		amfUeWriter, mongoapi.CommonDBClient = originalWriter, originalClient
		self.EnableDbStore, self.Drsm = originalStore, originalDrsm
		self.UePool.Delete(supi)
		for _, queue := range w.queues {
			close(queue)
		}
	})
	w.start()
}

// otherSupi returns a SUPI other than supi whose releases move the same shard of unmarked,
// or a different one.
func otherSupi(t *testing.T, supi string, sameShard bool) string {
	t.Helper()

	for i := range 100000 {
		candidate := fmt.Sprintf("imsi-20893019%07d", i)
		if candidate != supi && (unmarkedShard(candidate) == unmarkedShard(supi)) == sameShard {
			return candidate
		}
	}
	t.Fatal("no SUPI found")

	return ""
}

// runRelease marks and clears supi as a release does, without a UE to remove.
func runRelease(supi string) {
	markBeingDeleted(supi)
	clearBeingDeleted(supi)
}

// A release that finishes while the lookup reads again must still stop the publication.
// Another release in the same shard finishes during the first read, so the lookup reads
// again, and this UE's release runs to completion during that second read, after it has
// taken the document. A lookup that published what that read returned without checking
// again would restore the deleted context. Two lookups for the same UE reach the same
// point when the second waits for dbMutex while the first publishes and a release runs;
// that wait cannot be observed without a sleep, and the read can.
func TestAReleaseFinishingDuringTheSecondReadStopsThePublication(t *testing.T) {
	const supi = "imsi-208930100009803"
	self := AMF_Self()
	fake := &deletingDB{
		doc:           storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}),
		deleteEntered: make(chan struct{}),
		releaseDelete: make(chan struct{}),
	}
	close(fake.releaseDelete)
	installDeletingDB(t, fake, supi)

	ue := storableUe(supi)
	neighbour := otherSupi(t, supi, true)
	reads := 0
	fake.afterRead = func() {
		reads++
		switch reads {
		case 1:
			runRelease(neighbour)
		case 2:
			// The second has taken the document. A lookup on another association
			// publishes this UE, and its release then runs to completion.
			self.UePool.Store(supi, ue)
			RemoveUeAndDeleteItsContext(ue)
		}
	}

	restored, found := self.AmfUeFindBySupi(supi)

	if reads < 2 {
		t.Fatalf("the lookup read %d times; the release never ran inside it", reads)
	}
	if found {
		t.Fatalf("a lookup whose second read preceded the delete restored the deleted context (%p)", restored)
	}
	if _, pooled := self.UePool.Load(supi); pooled {
		t.Error("the released UE is back in the pool")
	}
}

// A lookup that reads again because another UE in its shard was released during its
// read still restores the UE once a read comes back with nothing moved.
func TestALookupThatReadsAgainStillRestoresTheUe(t *testing.T) {
	const supi = "imsi-208930100009805"
	self := AMF_Self()
	fake := &deletingDB{
		doc:           storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}),
		deleteEntered: make(chan struct{}),
		releaseDelete: make(chan struct{}),
	}
	installDeletingDB(t, fake, supi)

	neighbour := otherSupi(t, supi, true)
	reads := 0
	fake.afterRead = func() {
		reads++
		if reads == 1 {
			runRelease(neighbour)
		}
	}

	restored, found := self.AmfUeFindBySupi(supi)

	if reads != 2 {
		t.Fatalf("the lookup read %d times, want 2", reads)
	}
	if !found || restored == nil {
		t.Fatal("the lookup did not restore the UE after reading again")
	}
	if pooled, ok := self.UePool.Load(supi); !ok || pooled != restored {
		t.Error("the restored UE is not the one in the pool")
	}
}

// A release of a UE in another shard does not make a lookup read again.
func TestAReleaseInAnotherShardDoesNotMakeTheLookupReadAgain(t *testing.T) {
	const supi = "imsi-208930100009806"
	self := AMF_Self()
	fake := &deletingDB{
		doc:           storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}),
		deleteEntered: make(chan struct{}),
		releaseDelete: make(chan struct{}),
	}
	installDeletingDB(t, fake, supi)

	stranger := otherSupi(t, supi, false)
	reads := 0
	fake.afterRead = func() {
		reads++
		runRelease(stranger)
	}

	restored, found := self.AmfUeFindBySupi(supi)

	if reads != 1 {
		t.Fatalf("the lookup read %d times; a release in another shard made it read again", reads)
	}
	if !found || restored == nil {
		t.Fatal("the lookup did not restore the UE")
	}
}

// A lookup whose shard moves during every read gives up after restoreAttempts reads, and
// treats the UE as absent rather than publish a document a release may have deleted.
func TestALookupGivesUpWhenEveryReadSeesARelease(t *testing.T) {
	const supi = "imsi-208930100009807"
	self := AMF_Self()
	fake := &deletingDB{
		doc:           storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}),
		deleteEntered: make(chan struct{}),
		releaseDelete: make(chan struct{}),
	}
	installDeletingDB(t, fake, supi)

	neighbour := otherSupi(t, supi, true)
	reads := 0
	fake.afterRead = func() {
		reads++
		runRelease(neighbour)
	}

	restored, found := self.AmfUeFindBySupi(supi)

	if reads != restoreAttempts {
		t.Fatalf("the lookup read %d times, want %d", reads, restoreAttempts)
	}
	if found {
		t.Fatalf("a lookup that never read without a release restored the context (%p)", restored)
	}
	if _, pooled := self.UePool.Load(supi); pooled {
		t.Error("the lookup published the UE")
	}
}

// Two releases of one SUPI can overlap -- one per access, say. The first to finish must
// not clear the mark the second still holds.
func TestOneOfTwoOverlappingReleasesDoesNotClearTheOthersMark(t *testing.T) {
	const supi = "imsi-208930100009808"
	self := AMF_Self()
	fake := &deletingDB{
		doc:           storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}),
		deleteEntered: make(chan struct{}),
		releaseDelete: make(chan struct{}),
	}
	installDeletingDB(t, fake, supi)

	markBeingDeleted(supi)
	markBeingDeleted(supi)
	clearBeingDeleted(supi)
	t.Cleanup(func() { clearBeingDeleted(supi) })

	restored, found := self.AmfUeFindBySupi(supi)

	if found {
		t.Fatalf("a lookup restored the context while a second release of it was still deleting it (%p)", restored)
	}
	if _, pooled := self.UePool.Load(supi); pooled {
		t.Error("the lookup published the UE")
	}
}

// A lookup by SUPI that restores a UE must not publish it again once DbFetch has returned:
// a release that runs in between would find its UE back in the pool. The release is run
// from the one line between the two, the lookup's own log of what it found.
func TestALookupBySupiDoesNotPublishTheRestoredUeAgain(t *testing.T) {
	const supi = "imsi-208930100009804"
	self := AMF_Self()
	fake := &deletingDB{
		doc:           storedUeWithTriggers(t, supi, []models.RequestTrigger{models.REQUESTTRIGGER_LOC_CH}),
		deleteEntered: make(chan struct{}),
		releaseDelete: make(chan struct{}),
	}
	close(fake.releaseDelete)
	installDeletingDB(t, fake, supi)

	released := 0
	discard := zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(io.Discard), zapcore.InfoLevel)
	originalLog := logger.ContextLog
	logger.ContextLog = zap.New(zapcore.RegisterHooks(discard, func(entry zapcore.Entry) error {
		if released == 0 && strings.Contains(entry.Message, "found in DB") {
			released++
			if value, pooled := self.UePool.Load(supi); pooled {
				RemoveUeAndDeleteItsContext(value.(*AmfUe))
			}
		}
		return nil
	})).Sugar()
	t.Cleanup(func() { logger.ContextLog = originalLog })

	restored, found := self.AmfUeFindBySupi(supi)

	if released != 1 {
		t.Fatal("the lookup never logged what it found, so the release never ran in between")
	}
	if !found || restored == nil {
		t.Fatal("the lookup did not restore the stored context")
	}
	if _, pooled := self.UePool.Load(supi); pooled {
		t.Error("the lookup published the UE again after its release had removed it")
	}
}
