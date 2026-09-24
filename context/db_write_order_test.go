// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"slices"
	"testing"
	"time"

	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// orderDB records the writes that reach the datastore, in the order they land. A
// store can be held in flight: it signals on storeEntered and waits on releaseStore
// before it counts as landed. Every other DBInterface method is promoted from a nil
// interface, so a write path that reaches for one panics rather than passing.
type orderDB struct {
	mongoapi.DBInterface
	landed       chan string
	storeEntered chan string
	releaseStore chan struct{}
}

func newOrderDB() *orderDB {
	return &orderDB{landed: make(chan string, 16)}
}

// holdStores makes each store wait until releaseStore is closed.
func (f *orderDB) holdStores() {
	f.storeEntered = make(chan string, 16)
	f.releaseStore = make(chan struct{})
}

func (f *orderDB) RestfulAPIPost(_ string, filter bson.M, _ map[string]any) (bool, error) {
	supi, _ := filter["supi"].(string)
	if f.storeEntered != nil {
		f.storeEntered <- supi
		<-f.releaseStore
	}
	f.landed <- "store " + supi

	return true, nil
}

func (f *orderDB) RestfulAPIDeleteOne(_ string, filter bson.M) error {
	supi, _ := filter["supi"].(string)
	f.landed <- "delete " + supi

	return nil
}

// awaitLanded collects n landed writes, failing if they do not all arrive.
func (f *orderDB) awaitLanded(t *testing.T, n int) []string {
	t.Helper()

	var got []string
	for range n {
		select {
		case op := <-f.landed:
			got = append(got, op)
		case <-time.After(5 * time.Second):
			t.Fatalf("landed %v, then nothing: want %d writes", got, n)
		}
	}

	return got
}

// withWriter installs a fresh, started writer and a fake datastore for one test,
// with the datastore enabled so StoreContextInDB and DeleteContextFromDB reach them.
func withWriter(t *testing.T, workers, queueSize int, fake *orderDB) {
	t.Helper()

	w := newDBWriter(workers, queueSize)
	previousWriter, previousClient := amfUeWriter, mongoapi.CommonDBClient
	previousEnabled := AMF_Self().EnableDbStore
	amfUeWriter, mongoapi.CommonDBClient = w, fake
	AMF_Self().EnableDbStore = true
	t.Cleanup(func() {
		amfUeWriter, mongoapi.CommonDBClient = previousWriter, previousClient
		AMF_Self().EnableDbStore = previousEnabled
		for _, queue := range w.queues {
			close(queue)
		}
	})
	w.start()
}

func storableUe(supi string) *AmfUe {
	ue := &AmfUe{}
	ue.init()
	ue.SetSupi(supi)

	return ue
}

// deleteInBackground runs DeleteContextFromDB and returns a channel closed when it
// returns.
func deleteInBackground(ue *AmfUe) chan struct{} {
	deleted := make(chan struct{})
	go func() {
		DeleteContextFromDB(ue)
		close(deleted)
	}()

	return deleted
}

// stillWaiting reports whether deleted is still open after a short grace period.
func stillWaiting(deleted chan struct{}) bool {
	select {
	case <-deleted:
		return false
	case <-time.After(100 * time.Millisecond):
		return true
	}
}

// The race this fixes: a store queued before a delete, still in flight when the
// delete was made, landed after it and recreated the document -- the delete ran at
// once on the caller's goroutine while the store sat with a worker.
//
// The delete also stays synchronous: it returns only once it has run. NGAP handles
// an association's messages one at a time, so that is what keeps the next message
// from the same gNB from reading the document back.
func TestDeleteLandsAfterAStoreStillInFlight(t *testing.T) {
	fake := newOrderDB()
	fake.holdStores()
	withWriter(t, dbWriteWorkers, dbWriteQueueSize, fake)

	ue := storableUe("imsi-208930100009301")
	StoreContextInDB(ue)
	<-fake.storeEntered

	deleted := deleteInBackground(ue)
	if !stillWaiting(deleted) {
		t.Fatal("DeleteContextFromDB returned while a store queued ahead of it was still in flight")
	}
	close(fake.releaseStore)
	<-deleted

	want := []string{"store imsi-208930100009301", "delete imsi-208930100009301"}
	var got []string
	for len(got) < len(want) {
		select {
		case op := <-fake.landed:
			got = append(got, op)
		default:
			t.Fatalf("DeleteContextFromDB returned with only %v landed, want %v", got, want)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("landed %v, want %v: a store queued before the delete landed after it and "+
			"recreated the document", got, want)
	}
}

// Nothing supersedes a dropped delete, so a full queue is waited on rather than
// dropped. A dropped store is different: the UE's next store replaces it.
func TestDeleteWaitsForRoomRatherThanBeingDropped(t *testing.T) {
	fake := newOrderDB()
	fake.holdStores()
	// One worker and a queue of one, so the queue is full with a single store waiting.
	withWriter(t, 1, 1, fake)

	first := storableUe("imsi-208930100009303")
	StoreContextInDB(first)
	<-fake.storeEntered
	StoreContextInDB(storableUe("imsi-208930100009304"))

	deleted := deleteInBackground(first)
	if !stillWaiting(deleted) {
		t.Fatal("DeleteContextFromDB returned while the queue was full; it can only have dropped the delete")
	}
	close(fake.releaseStore)

	want := []string{
		"store imsi-208930100009303", "store imsi-208930100009304", "delete imsi-208930100009303",
	}
	if got := fake.awaitLanded(t, 3); !slices.Equal(got, want) {
		t.Fatalf("landed %v, want %v", got, want)
	}
	<-deleted
}
