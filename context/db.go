// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/maphash"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/omec-project/amf/factory"
	"github.com/omec-project/amf/logger"
	"github.com/omec-project/amf/metrics"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/util/idgenerator"
	"github.com/omec-project/util/mongoapi"
	"go.mongodb.org/mongo-driver/v2/bson"
)

var dbMutex sync.Mutex

const (
	dbWriteWorkers = 4
	// dbWriteQueueSize is the capacity of all the write queues together; each
	// worker's queue holds its share of it.
	dbWriteQueueSize = 256
)

const (
	// indexEnsureBudget bounds the whole of ensureIndexes. It is generous
	// because the usual reason to need it is that MongoDB elects a primary later
	// than the AMF started, and giving up before that happens only restarts the
	// same wait.
	indexEnsureBudget = 5 * time.Minute
	// indexEnsureAttemptTimeout bounds one EnsureIndex call, which builds the
	// index and then reads the collection's indexes back to confirm it.
	indexEnsureAttemptTimeout = 30 * time.Second
	indexEnsureInitialBackoff = 1 * time.Second
	indexEnsureMaxBackoff     = 30 * time.Second

	// mongoConnectAttempts bounds the connect loop. ConnectMongo waits three
	// minutes per attempt, so this is a nine-minute ceiling before the AMF
	// reports that it has no datastore.
	mongoConnectAttempts = 3
)

// dbWriteOp is one write to amf.data.amfState: an upsert of data or, when
// removed is set, a delete, which closes removed once it has run. Either way it
// is matched on filter, the UE's supi.
type dbWriteOp struct {
	filter  bson.M
	data    bson.M
	removed chan struct{}
}

// dbWriter runs the writes to amf.data.amfState off the caller's goroutine,
// in order for each UE.
//
// Every write for a UE goes to the one queue its supi selects, and each queue
// has a single worker, so a UE's writes land in the order they were made. That
// is what makes a delete final. With one queue shared by every worker, a store
// queued before the delete could still be waiting, or in flight on another
// worker, when the delete ran -- and it would then recreate the document the
// delete had just removed.
type dbWriter struct {
	queues []chan dbWriteOp
	seed   maphash.Seed
	once   sync.Once
}

func newDBWriter(workers, queueSize int) *dbWriter {
	w := &dbWriter{queues: make([]chan dbWriteOp, workers), seed: maphash.MakeSeed()}
	for i := range w.queues {
		w.queues[i] = make(chan dbWriteOp, queueSize/workers)
	}
	return w
}

var amfUeWriter = newDBWriter(dbWriteWorkers, dbWriteQueueSize)

func startDBWriteWorkers() {
	amfUeWriter.start()
}

func (w *dbWriter) start() {
	w.once.Do(func() {
		for _, queue := range w.queues {
			go func() {
				for op := range queue {
					if op.removed != nil {
						if delErr := mongoapi.CommonDBClient.RestfulAPIDeleteOne(AmfUeDataColl, op.filter); delErr != nil {
							logger.DataRepoLog.Warnln(delErr)
						}
						close(op.removed)
						continue
					}
					if _, postErr := mongoapi.CommonDBClient.RestfulAPIPost(AmfUeDataColl, op.filter, op.data); postErr != nil {
						logger.DataRepoLog.Warnln(postErr)
					}
				}
			}()
		}
	})
}

func (w *dbWriter) queueFor(supi string) chan dbWriteOp {
	return w.queues[maphash.String(w.seed, supi)%uint64(len(w.queues))]
}

// store queues an upsert, and reports false if the queue was full and the
// write was dropped. A dropped store is superseded by the UE's next one.
func (w *dbWriter) store(supi string, data bson.M) bool {
	select {
	case w.queueFor(supi) <- dbWriteOp{filter: bson.M{"supi": supi}, data: data}:
		return true
	default:
		return false
	}
}

// remove queues a delete behind every write already queued for the UE and
// returns once it has run. A full queue is waited on rather than dropped:
// nothing supersedes a dropped delete, so the document would stay.
func (w *dbWriter) remove(supi string) {
	op := dbWriteOp{filter: bson.M{"supi": supi}, removed: make(chan struct{})}
	w.queueFor(supi) <- op
	<-op.removed
}

type CustomFieldsAmfUe struct {
	State       map[models.AccessType]string `json:"state"`
	SmCtxList   map[string]SmContext         `json:"smCtxList"`
	N1N2Message *N1N2Message                 `json:"n1n2Msg,omitempty"`
	ULCount     uint32                       `json:"ulCount"`
	DLCount     uint32                       `json:"dlCount"`
	RanUeNgapId int64                        `json:"ranUeNgapId"`
	AmfUeNgapId int64                        `json:"amfUeNgapId"`
	RanId       string                       `json:"ranId"`
}

var (
	Namespace     = os.Getenv("POD_NAMESPACE")
	AmfUeDataColl = "amf.data.amfState"
)

func AllocateUniqueID(generator **idgenerator.IDGenerator, idName string) (int64, error) {
	// Use MongoDB increment field to generate new offset.
	// generate ids between offset to 8192 above offset.
	dbMutex.Lock()
	defer dbMutex.Unlock()
	if *generator == nil {
		if !datastoreReady() {
			return -1, errors.New("no connection to MongoDB, so no id range could be claimed")
		}
		logger.DataRepoLog.Infof("generator null. fetch offset from db")
		val := mongoapi.CommonDBClient.GetUniqueIdentity(idName)
		// Mongodb returns value starting from 1.
		// Limiting users to 8192(2^13) per instance.
		// TODO : Make this value configurable.
		//        Later this value can be used to trigger
		//        creation of new instance
		minVal := int64((val-1)*8192 + 1)
		maxVal := minVal + 8192
		*generator = idgenerator.NewGenerator(minVal, maxVal)
	}

	val, err := (*generator).Allocate()
	if err != nil {
		logger.DataRepoLog.Warnf("Max IDs generated for Instance")
		return -1, err
	}

	return val, nil
}

// redactedMongoURL renders a connection string down to the part of it this log
// line exists to record: which server, and which database.
//
// It builds that string rather than editing the one it was given, which is the
// only version of this that is safe by construction: the result can hold
// nothing that is not put into it by name. Redaction in place was tried twice
// and leaked twice. url.Redacted covers the userinfo password and leaves the
// query alone, where a MongoDB connection string also keeps
// tlsCertificateKeyFilePassword, sslClientCertificateKeyPassword and
// authMechanismProperties -- the last holding things like an AWS session token,
// all of them matched case-insensitively by a driver that gains options between
// releases. And url.Parse accepts a string with no authority at all: "user:pw@h"
// parses as scheme "user" with the rest opaque, which Redacted then returns
// verbatim, password included.
//
// So anything without a host is reported as unparseable rather than printed,
// and what is printed is the scheme, the host, the path and a username -- never
// a password value, never the query, never an opaque remainder.
func redactedMongoURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "(unparseable connection string)"
	}

	safe := url.URL{Scheme: parsed.Scheme, Host: parsed.Host, Path: parsed.Path}
	if parsed.User != nil {
		if _, hasPassword := parsed.User.Password(); hasPassword {
			safe.User = url.UserPassword(parsed.User.Username(), "xxxxx")
		} else {
			safe.User = url.User(parsed.User.Username())
		}
	}
	return safe.String()
}

func SetupAmfCollection() error {
	mongoDbUrl := "mongodb://mongodb:27017"
	if factory.AmfConfig.Configuration.AmfDBName == "" {
		factory.AmfConfig.Configuration.AmfDBName = "sdcore_amf"
	}

	if (factory.AmfConfig.Configuration.Mongodb != nil) &&
		(factory.AmfConfig.Configuration.Mongodb.Url != "") {
		mongoDbUrl = factory.AmfConfig.Configuration.Mongodb.Url
	}

	logger.DataRepoLog.Infof("MongoDbName: %v, Url: %v", factory.AmfConfig.Configuration.AmfDBName, redactedMongoURL(mongoDbUrl))

	if Namespace != "" {
		AmfUeDataColl = Namespace + "." + AmfUeDataColl
	}
	// Bounded, where this used to retry for ever. ConnectMongo already waits
	// three minutes per attempt, so a handful of them is a long time to sit in
	// boot with nothing to show for it, and an AMF that will not get a
	// datastore should say so rather than hang. Reporting it lets the caller
	// decide, which for this one is to end the process.
	connected := false
	for attempt := 1; attempt <= mongoConnectAttempts; attempt++ {
		mongoapi.ConnectMongo(mongoDbUrl, factory.AmfConfig.Configuration.AmfDBName)
		// ConnectMongo assigns CommonDBClient only once it has connected, so
		// after it gives up the interface is still nil -- and a plain type
		// assertion on a nil interface panics, which would end the process
		// before any of the reporting below could run.
		client, isMongo := mongoapi.CommonDBClient.(*mongoapi.MongoClient)
		if isMongo && client.Client != nil {
			logger.DataRepoLog.Infoln("successfully connected to Mongodb")
			connected = true
			break
		}
		logger.DataRepoLog.Errorf("mongoDb connection failed, attempt %d of %d", attempt, mongoConnectAttempts)
	}
	if !connected {
		// Without the URL: it is a connection string, and one carrying
		// credentials would otherwise reach the logs through the caller that
		// prints this.
		return fmt.Errorf("no connection to MongoDB after %d attempts", mongoConnectAttempts)
	}
	ctx, cancel := context.WithTimeout(context.Background(), indexEnsureBudget)
	defer cancel()

	return prepareCollection(ctx, amfUeIndexes(), startDBWriteWorkers)
}

// prepareCollection puts every index in place and only then lets writers run.
//
// The order is the point, which is why it is a function of its own rather than
// two statements: writes here are upserts, so two workers writing the same key
// concurrently can each insert while no unique index covers it -- and a
// collection that has acquired such a pair can never have that index created
// again, on this start or any later one. For an AMF that refuses to run without
// it, that is a permanent failure to start. It applies to the drop and recreate
// that changing an existing index's options requires, not only to a first
// creation.
//
// Nothing is enqueued while the indexes are ensured, either: service/init.go
// calls this before it starts the NGAP listener, and every caller of
// StoreContextInDB is a GMM or NGAP handler. So the wait costs nothing even
// when MongoDB has not elected a primary yet and it takes minutes.
func prepareCollection(ctx context.Context, specs []mongoapi.IndexSpec, startWriters func()) error {
	if err := ensureIndexes(ctx, specs); err != nil {
		return err
	}

	startWriters()

	return nil
}

// amfUeIndexes are the indexes amf.data.amfState needs, one per filter the AMF
// queries or writes that collection by. Every one of them is a seek the AMF
// would otherwise do as a collection scan: a write is an upsert matched on
// supi, and the three fetch paths filter on guti, on (ranId, ranUeNgapId) and
// on amfUeNgapId.
//
// Uniqueness is asserted only where the field really carries it. supi
// identifies the document and is the filter every write upserts on. guti and
// tmsi are unique per UE but `omitempty`, so they are absent from a context
// stored before one was assigned -- a unique index reads every such document as
// null and rejects the second one with a duplicate key error, which the write
// worker can only log. Sparse is what keeps the constraint without the
// collision.
//
// The two RAN identifiers carry no uniqueness at all: ranUeNgapId is assigned
// by the gNB and repeats across gNBs, which is why indexing it alone was
// abandoned. They are also written as zero rather than omitted for a UE with no
// current RAN association, so sparse cannot exclude those either -- the field is
// present, it just holds a placeholder. A partial filter is what excludes them,
// and it keeps the index to the documents a lookup can actually want.
func amfUeIndexes() []mongoapi.IndexSpec {
	// A context stored with no RAN association writes ranId as the empty
	// string alongside both NGAP IDs as zero (see AmfUe.MarshalJSON), so this
	// selects exactly the UEs a lookup by RAN identifier can return.
	attachedToARan := bson.M{"customFieldsAmfUe.ranId": bson.M{"$gt": ""}}

	return []mongoapi.IndexSpec{
		amfUeSupiIndex(),
		{
			Name:   "amfUeByGuti",
			Keys:   mongoapi.AscendingKeys("guti"),
			Unique: true,
			Sparse: true,
		},
		{
			Name:   "amfUeByTmsi",
			Keys:   mongoapi.AscendingKeys("tmsi"),
			Unique: true,
			Sparse: true,
		},
		{
			// Keyed ranId first so the index also answers "every UE on this
			// gNB", which NG Reset and RAN-wide cleanup want; both predicates
			// are equality, so either order serves DbFetchRanUeByRanUeNgapID
			// equally well and the prefix is the only thing the choice buys.
			Name:          "amfUeByRan",
			Keys:          mongoapi.AscendingKeys("customFieldsAmfUe.ranId", "customFieldsAmfUe.ranUeNgapId"),
			PartialFilter: attachedToARan,
		},
		{
			// Filtered on the field itself rather than on ranId: a partial
			// index is only used when the query proves the filter, and a query
			// on amfUeNgapId alone proves nothing about ranId. Zero is excluded
			// because zero is what MarshalJSON writes for a UE with no RAN
			// association, which is the overwhelming majority of the documents
			// carrying it.
			//
			// Not quite all of them, though: with EnableDbStore the ID comes
			// from drsm, which composes it as (chunk << 10) | offset over a
			// chunk drawn from 16384 and offsets counted down from 999, so an
			// AMF that draws chunk zero and exhausts it allocates a real UE the
			// value zero. That UE's own lookup then falls back to a collection
			// scan rather than returning a wrong answer, since the document is
			// only missing from the index and not from the collection. The
			// deeper problem there is that its context is indistinguishable
			// from a detached one in the stored data itself, which no index can
			// fix.
			Name:          "amfUeByAmfUeNgapId",
			Keys:          mongoapi.AscendingKeys("customFieldsAmfUe.amfUeNgapId"),
			PartialFilter: bson.M{"customFieldsAmfUe.amfUeNgapId": bson.M{"$gt": 0}},
		},
	}
}

// amfUeSupiIndex is the index over the field every write upserts on.
//
// Its specification is deliberately identical to what the CreateIndex call this
// change replaces already produced -- key {supi: 1}, unique, nothing else, under
// the name MongoDB generates for that key -- so that ensuring it on a collection
// that has one is a no-op rather than a drop and recreate.
//
// That matters because the AMF is deployed as several instances against one
// collection, and nothing here can stop the others writing. For guti and tmsi a
// replacement window is worth it: they are unique over `omitempty` fields today,
// which rejects the second UE that has no such value, and sparse is the fix. For
// supi it is not. Every write is an upsert whose filter is {supi: ...}, and
// MongoDB copies a filter's equality fields into any document it inserts, so no
// document this code writes can lack the field -- sparse would guard against
// something unreachable, at the price of dropping the one index that keeps
// concurrent upserts from making two documents for one UE.
func amfUeSupiIndex() mongoapi.IndexSpec {
	return mongoapi.IndexSpec{
		Name:   "supi_1",
		Keys:   mongoapi.AscendingKeys("supi"),
		Unique: true,
	}
}

// ensureIndexes makes amf.data.amfState carry every index given, and reports
// the first one it cannot.
//
// Creating an index is a write, so it fails while the replica set has no
// writable primary yet -- routine when the AMF and MongoDB start together --
// which is why this retries rather than logging once and carrying on. Carrying
// on is what it must not do: every fetch below degrades to a collection scan
// whose cost grows with the number of subscribers, and DbFetchRanUeByRanUeNgapID
// runs that scan while holding the RAN's state lock, so one missing index
// stalls a whole gNB association. None of that is visible in a log line.
//
// It reports rather than exits because the decision is the caller's, not this
// function's -- and because a returned error is testable where ending the
// process is not: a Fatalf here would take the test binary with it, leaving the
// failure path uncovered.
func ensureIndexes(ctx context.Context, specs []mongoapi.IndexSpec) error {
	for _, spec := range specs {
		if err := ensureIndexWithRetry(ctx, spec); err != nil {
			return fmt.Errorf("could not ensure index %q on collection %q: %w",
				spec.Name, AmfUeDataColl, err)
		}
		logger.DataRepoLog.Infof("index %q is present on collection %q", spec.Name, AmfUeDataColl)
	}
	return nil
}

// ensureIndexWithRetry calls EnsureIndex until it succeeds or ctx expires,
// backing off between attempts.
func ensureIndexWithRetry(ctx context.Context, spec mongoapi.IndexSpec) error {
	backoff := indexEnsureInitialBackoff
	for attempt := 1; ; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, indexEnsureAttemptTimeout)
		err := mongoapi.CommonDBClient.EnsureIndex(attemptCtx, AmfUeDataColl, spec)
		cancel()
		if err == nil {
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("gave up after %d attempts: %w (last error: %v)", attempt, ctxErr, err)
		}
		logger.DataRepoLog.Warnf("attempt %d to ensure index %q on collection %q failed, retrying in %s: %v",
			attempt, spec.Name, AmfUeDataColl, backoff, err)
		select {
		case <-ctx.Done():
			return fmt.Errorf("gave up after %d attempts: %w (last error: %v)", attempt, ctx.Err(), err)
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, indexEnsureMaxBackoff)
	}
}

// amfJSONBufInitialCap is the initial capacity for pooled JSON encoding buffers
// (32 KiB covers typical UE context sizes without frequent reallocation).
const amfJSONBufInitialCap = 32 * 1024

// amfJSONBufPool pools the bytes.Buffer used for sonic encoding to reduce GC pressure.
var amfJSONBufPool = sync.Pool{
	New: func() any { return bytes.NewBuffer(make([]byte, 0, amfJSONBufInitialCap)) },
}

func ToBsonM(data *AmfUe) (ret bson.M) {
	buf := amfJSONBufPool.Get().(*bytes.Buffer)
	buf.Reset()
	enc := sonic.ConfigDefault.NewEncoder(buf)
	if err := enc.Encode(data); err != nil {
		amfJSONBufPool.Put(buf)
		logger.DataRepoLog.Errorf("amfue marshal error: %v", err)
		return
	}
	if err := sonic.Unmarshal(buf.Bytes(), &ret); err != nil {
		logger.DataRepoLog.Errorf("amfue unmarshal error: %v", err)
	}
	amfJSONBufPool.Put(buf)
	return
}

func StoreContextInDB(ue *AmfUe) {
	self := AMF_Self()
	if !self.EnableDbStore {
		return
	}
	// Serialize synchronously (snapshot before next EventChannel message can modify ue).
	amfUeBsonA := ToBsonM(ue)
	if amfUeBsonA == nil {
		return
	}
	if !amfUeWriter.store(ue.GetSupi(), amfUeBsonA) {
		metrics.IncrementDbWriteDropped()
		logger.DataRepoLog.Warnf("DB write queue full, dropping store for supi=%s", ue.GetSupi())
	}
}

// DeleteContextFromDB deletes the UE's stored context behind every store
// already queued for it, so that none of them can land afterwards, and returns
// once the delete has run. It waits for the stores ahead of it on the same
// queue, where it used to run at once and race them.
func DeleteContextFromDB(ue *AmfUe) {
	self := AMF_Self()
	if self.EnableDbStore {
		if !datastoreReady() {
			return
		}
		amfUeWriter.remove(ue.GetSupi())
	}
}

// dropEmptyEnumValues removes keys whose value is an empty string, in place, walking
// nested objects and arrays.
//
// Decoding an object into Go treats an absent key and an empty string identically -- the
// field is left at its zero value -- with one exception: the strict 3GPP enum decoders
// refuse an empty value, and refuse the whole document with it. A stored context that is
// only partly populated therefore becomes unreadable, and it does not take much: an
// optional container written with no class, or ngKsi.tsc on a UE stored before
// authentication finished.
//
// This runs only after a strict decode has already failed, so a well-formed record is
// never touched by it.
func dropEmptyEnumValues(value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, held := range typed {
			if str, isString := held.(string); isString && str == "" {
				delete(typed, key)
				continue
			}

			dropEmptyEnumValues(held)
		}
	case []any:
		for _, held := range typed {
			dropEmptyEnumValues(held)
		}
	}
}

// datastoreReady reports whether there is a client to call.
//
// SetupAmfCollection is the only thing that sets one, and it runs only when
// EnableDbStore is configured -- while not every path that reads the datastore
// checks that flag first. HandleOAMActiveUEContextsFromDB is the plain example:
// it calls DbFetchAllEntries for any operator that asks, whether or not the AMF
// was configured with somewhere to fetch from. A method call on a nil interface
// panics rather than failing, so each of those paths asks here instead.
//
// This is not about the startup window. Setup runs synchronously before the
// NGAP listener and the SBI server (see service/init.go), so nothing races it
// and nothing serves ahead of it.
func datastoreReady() bool {
	if mongoapi.CommonDBClient == nil {
		logger.DataRepoLog.Warnln("no connection to MongoDB yet, skipping the datastore")
		return false
	}
	return true
}

func DbFetch(collName string, filter bson.M) *AmfUe {
	if !datastoreReady() {
		return nil
	}
	ue := &AmfUe{}
	ue.init()
	result, getOneErr := mongoapi.CommonDBClient.RestfulAPIGetOne(collName, filter)
	if getOneErr != nil {
		logger.DataRepoLog.Warnln(getOneErr)
	}

	if len(result) == 0 {
		return nil
	}

	err := sonic.Unmarshal(mapToByte(result), ue)
	if err != nil {
		// Retry without the empty values a strict enum decoder refuses. Records written
		// before those values stopped being written are still in deployed databases,
		// and a UE whose record cannot be read is a UE that cannot be paged.
		strictErr := err

		dropEmptyEnumValues(result)

		ue = &AmfUe{}
		ue.init()

		if err = sonic.Unmarshal(mapToByte(result), ue); err != nil {
			// Not the same thing as an absent document, and conflating them is what
			// hid this for months: the context is there and unreadable, which is a
			// fault to fix rather than a subscriber to go looking for.
			logger.DataRepoLog.Errorf("stored UE context exists but could not be decoded: %v", err)

			return nil
		}

		logger.DataRepoLog.Warnf("read a stored UE context that a strict decode refused (%v); "+
			"empty values were dropped", strictErr)
	}

	dbMutex.Lock()
	defer dbMutex.Unlock()

	// Read once. This function publishes the UE to UePool before it returns, and nothing
	// serialises that against the other writers, so from that point the entry can be
	// replaced by another goroutine while this function is still using it.
	ranUe := ue.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS)
	if ranUe == nil {
		// A stored document without a 3GPP RanUe would otherwise be dereferenced below.
		// Callers already handle a nil return from this function.
		logger.DataRepoLog.Errorln("amfue restored without a 3GPP RanUe, discarding it")

		return nil
	}

	ranUe.SetAmfUe(ue)
	ue.EventChannel = nil
	ue.NASLog = logger.NasLog.With(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ranUe.AmfUeNgapId))
	ue.GmmLog = logger.GmmLog.With(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ranUe.AmfUeNgapId))
	ue.TxLog = logger.GmmLog.With(logger.FieldAmfUeNgapID, fmt.Sprintf("AMF_UE_NGAP_ID:%d", ranUe.AmfUeNgapId))
	ue.ProducerLog = logger.ProducerLog.With(logger.FieldSupi, fmt.Sprintf("SUPI:%s", ue.Supi))
	ue.AmfInstanceName = os.Getenv("HOSTNAME")
	ue.AmfInstanceIp = os.Getenv("POD_IP")

	// Published last, and this order is the point. Nothing above reads either pool, but
	// everything above is state a reader needs: until it is written, a lookup that finds
	// this UE gets a context with no loggers and no instance identity. AmfUeFindBySupi and
	// AmfUeFindByGuti do not hold dbMutex, so such a reader is a service request arriving
	// while the restore is still running -- and since a request for a UE with no event
	// channel now runs its handler inline rather than failing, that reader reaches the
	// procedure and the per-UE loggers it uses.
	AMF_Self().RanUePool.Store(ranUe.AmfUeNgapId, ranUe)
	AMF_Self().UePool.Store(ue.Supi, ue)

	ue.TxLog.Debugln("amfue fetched")
	return ue
}

func DbFetchRanUeByRanUeNgapID(ranUeNgapID int64, ran *AmfRan) *RanUe {
	filter := bson.M{}
	filter["customFieldsAmfUe.ranUeNgapId"] = ranUeNgapID
	filter["customFieldsAmfUe.ranId"] = ran.GnbId

	ue := DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.DataRepoLog.Debugln("DbFetchRanUeByRanUeNgapID: no document found for ranUeNgapID", ranUeNgapID)
		return nil
	}

	// Lock order, because this is where the two meet: the caller holds ran.ranStateMu across
	// this call, and the accessor below takes ue.Mutex. Nothing acquires them the other way
	// round today -- RanUe.Remove releases the UE lock before it takes the RAN one, and
	// RemoveAllUeInRan snapshots under a read lock and releases it before removing anything --
	// so this is an ordering to keep rather than a cycle to break. Taking ue.Mutex while holding
	// ran.ranStateMu is fine; taking ran.ranStateMu while holding ue.Mutex would not be.
	//
	// DbFetch above takes it once more, under dbMutex, making that call ran.ranStateMu ->
	// dbMutex -> ue.Mutex. That one cannot contend with anything: the UE it locks was
	// unmarshalled a few lines earlier and is not in RanUePool or UePool until after the call,
	// so no other goroutine holds a reference to it yet.
	//
	// Check if some parallel procedure has already
	// fetched AmfUe and stored the RanUE in context.
	// If so, then return the stored RanUE
	// else return RanUE from newly fetched AmfUe
	// and store in context
	ranUe := ran.RanUeFindByRanUeNgapIDLocal(ranUeNgapID)
	if ranUe != nil {
		return ranUe
	}
	return ue.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS)
}

func DbFetchRanUeByAmfUeNgapID(amfUeNgapID int64) *RanUe {
	self := AMF_Self()
	filter := bson.M{}
	filter["customFieldsAmfUe.amfUeNgapId"] = amfUeNgapID
	ue := DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.DataRepoLog.Errorln("DbFetchRanUeByAmfUeNgapID: no document found for amfUeNgapID ", amfUeNgapID)
		return nil
	}

	// Check if some parallel procedure has already
	// fetched AmfUe and stored the RanUE in context.
	// If so, then return the stored RanUE
	// else return RanUE from newly fetched AmfUe
	// and store in context
	ranUe := self.RanUeFindByAmfUeNgapIDLocal(amfUeNgapID)
	if ranUe != nil {
		return ranUe
	}
	return ue.GetRanUe(models.ACCESSTYPE__3_GPP_ACCESS)
}

func DbFetchUeByGuti(guti string) (ue *AmfUe, ok bool) {
	self := AMF_Self()
	filter := bson.M{}
	filter["guti"] = guti

	ue = DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.DataRepoLog.Warnln("FindByGuti: no document found for guti", guti)
		return nil, false
	} else {
		ok = true
	}

	// Check if some parallel procedure has already
	// fetched AmfUe. If so, then return the same.
	// else return newly fetched AmfUe and store in context
	if amfUe, ret := self.AmfUeFindByGutiLocal(guti); ret {
		logger.DataRepoLog.Infoln("FindByGuti: found by local", guti)
		ue = amfUe
		ok = ret
	}

	return ue, ok
}

func DbFetchUeBySupi(supi string) (ue *AmfUe, ok bool) {
	self := AMF_Self()
	filter := bson.M{}
	filter["supi"] = supi

	ue = DbFetch(AmfUeDataColl, filter)
	if ue == nil {
		logger.DataRepoLog.Warnln("FindBySupi: no document found for supi", supi)
		return nil, false
	} else {
		ok = true
	}
	// Check if some parallel procedure has already
	// fetched AmfUe. If so, then return the same.
	// else return newly fetched AmfUe and store in context
	if amfUe, ret := self.AmfUeFindBySupiLocal(supi); ret {
		logger.DataRepoLog.Infoln("FindBySupi: found by local", supi)
		ue = amfUe
		ok = ret
	}

	return ue, ok
}

func DbFetchAllEntries() (ueList []*AmfUe) {
	if !datastoreReady() {
		return nil
	}
	ue := &AmfUe{}
	filter := bson.M{}
	results, getManyErr := mongoapi.CommonDBClient.RestfulAPIGetMany(AmfUeDataColl, filter)
	if getManyErr != nil {
		logger.DataRepoLog.Warnln(getManyErr)
	}

	for _, val := range results {
		ue = &AmfUe{}
		ue.init()
		err := sonic.Unmarshal(mapToByte(val), ue)
		if err != nil {
			logger.DataRepoLog.Errorf("amfue unmarshal error: %v", err)
			return nil
		}
		ueList = append(ueList, ue)
	}

	return ueList
}
