// SPDX-FileCopyrightText: 2026 Forsway Scandinavia AB
// SPDX-License-Identifier: Apache-2.0

package context

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/omec-project/util/mongoapi"
)

// supiField is the field every write to amf.data.amfState upserts on.
const supiField = "supi"

// fakeDBClient answers EnsureIndex and nothing else. Every other method of
// DBInterface is inherited from a nil embedded interface, so a test that
// reaches one panics rather than quietly passing.
type fakeDBClient struct {
	mongoapi.DBInterface
	calls    []mongoapi.IndexSpec
	results  []error
	onEnsure func(mongoapi.IndexSpec)
}

func (f *fakeDBClient) EnsureIndex(_ context.Context, _ string, spec mongoapi.IndexSpec) error {
	f.calls = append(f.calls, spec)
	if f.onEnsure != nil {
		f.onEnsure(spec)
	}
	if len(f.results) == 0 {
		return nil
	}
	err := f.results[0]
	f.results = f.results[1:]
	return err
}

func withFakeDBClient(t *testing.T, fake *fakeDBClient) {
	t.Helper()
	previous := mongoapi.CommonDBClient
	mongoapi.CommonDBClient = fake
	t.Cleanup(func() { mongoapi.CommonDBClient = previous })
}

// indexedKeys renders a spec's keys the way a filter names its fields, so a
// test can compare the two directly.
func indexedKeys(spec mongoapi.IndexSpec) []string {
	fields := make([]string, 0, len(spec.Keys))
	for _, key := range spec.Keys {
		fields = append(fields, key.Key)
	}
	return fields
}

func specNamed(t *testing.T, name string) mongoapi.IndexSpec {
	t.Helper()
	for _, spec := range amfUeIndexes() {
		if spec.Name == name {
			return spec
		}
	}
	t.Fatalf("no index named %q", name)
	return mongoapi.IndexSpec{}
}

// TestAmfUeIndexesServeEveryFilterTheAmfQueriesBy pins the indexes to the
// filters in db.go. A filter no index serves scans the whole collection, and
// nothing at runtime says so.
//
// The rule it applies is MongoDB's: every field of the filter has to be among
// the leading keys of one index, with none of those leading keys left over.
// Order among them is deliberately not asserted -- every filter here is
// equality on each field, and an index over (a, b) serves {a, b} exactly as
// well as one over (b, a) does. Which order to prefer is a question about the
// prefix left over for other queries, not about whether these ones seek.
func TestAmfUeIndexesServeEveryFilterTheAmfQueriesBy(t *testing.T) {
	filters := map[string][]string{
		"StoreContextInDB / DeleteContextFromDB": {supiField},
		"DbFetchUeBySupi":                        {supiField},
		"DbFetchUeByGuti":                        {"guti"},
		"DbFetchRanUeByRanUeNgapID":              {"customFieldsAmfUe.ranId", "customFieldsAmfUe.ranUeNgapId"},
		"DbFetchRanUeByAmfUeNgapID":              {"customFieldsAmfUe.amfUeNgapId"},
	}

	specs := amfUeIndexes()
	for name, wanted := range filters {
		served := false
		for _, spec := range specs {
			keys := indexedKeys(spec)
			if len(keys) < len(wanted) {
				continue
			}
			leading := map[string]bool{}
			for _, field := range keys[:len(wanted)] {
				leading[field] = true
			}
			covered := true
			for _, field := range wanted {
				if !leading[field] {
					covered = false
					break
				}
			}
			if covered {
				served = true
				break
			}
		}
		if !served {
			t.Errorf("filter of %s on %v is served by no index, so it scans the collection", name, wanted)
		}
	}
}

// TestAmfUeIndexesAreNamedAndDistinct covers the two properties that make an
// index repairable later: an explicit name, and no two specs competing for one
// key pattern.
func TestAmfUeIndexesAreNamedAndDistinct(t *testing.T) {
	names := map[string]bool{}
	keyPatterns := map[string]string{}
	for _, spec := range amfUeIndexes() {
		if spec.Name == "" {
			t.Errorf("index over %v has no name, so a later version cannot replace it", indexedKeys(spec))
			continue
		}
		if names[spec.Name] {
			t.Errorf("index name %q is used twice", spec.Name)
		}
		names[spec.Name] = true

		key := ""
		for _, field := range indexedKeys(spec) {
			key += field + "\x00"
		}
		if previous, clash := keyPatterns[key]; clash {
			t.Errorf("indexes %q and %q both cover key pattern %v, so each replaces the other at startup",
				previous, spec.Name, indexedKeys(spec))
		}
		keyPatterns[key] = spec.Name
	}
}

// TestAmfUeIndexesAssertOnlyTheUniquenessTheDataHas is the regression test for
// the reason these indexes were absent: created unique, three of them reject
// writes the AMF makes legitimately.
func TestAmfUeIndexesAssertOnlyTheUniquenessTheDataHas(t *testing.T) {
	// guti and tmsi are `omitempty`, so a context stored before either was
	// assigned carries no such field. Unique without sparse reads every one of
	// them as null and rejects the second.
	for _, name := range []string{"amfUeByGuti", "amfUeByTmsi"} {
		spec := specNamed(t, name)
		if !spec.Unique {
			t.Errorf("%s: expected a unique index", name)
		}
		if !spec.Sparse {
			t.Errorf("%s: unique over an omitempty field without Sparse rejects the second UE that has no such value", name)
		}
	}

	// The RAN identifiers repeat across gNBs and are written as a placeholder
	// for a UE with no RAN association, so neither is unique on any terms.
	for _, name := range []string{"amfUeByRan", "amfUeByAmfUeNgapId"} {
		spec := specNamed(t, name)
		if spec.Unique {
			t.Errorf("%s: a unique index here rejects the second detached UE", name)
		}
		if spec.PartialFilter == nil {
			t.Errorf("%s: without a partial filter every detached UE sits in one bucket no query seeks", name)
		}
	}
}

// TestAmfUeIndexPartialFiltersAreProvableByTheQuery covers the way a partial
// index fails silently: MongoDB uses one only when the query's own predicate
// implies the filter, so a filter over a field the query does not mention
// leaves the lookup scanning with nothing to show for the index.
func TestAmfUeIndexPartialFiltersAreProvableByTheQuery(t *testing.T) {
	byAmfUeNgapID := specNamed(t, "amfUeByAmfUeNgapId")
	if _, filtersOnItsOwnKey := byAmfUeNgapID.PartialFilter["customFieldsAmfUe.amfUeNgapId"]; !filtersOnItsOwnKey {
		t.Errorf("amfUeByAmfUeNgapId is filtered on %v, which a lookup by amfUeNgapId alone cannot prove, so the index goes unused",
			byAmfUeNgapID.PartialFilter)
	}

	// The compound index is queried with equality on both fields, so a filter
	// over either one is provable. It uses ranId because that is the field
	// whose placeholder marks a detached UE.
	byRan := specNamed(t, "amfUeByRan")
	if _, filtersOnRanID := byRan.PartialFilter["customFieldsAmfUe.ranId"]; !filtersOnRanID {
		t.Errorf("amfUeByRan is filtered on %v rather than on ranId, the field that marks a detached context",
			byRan.PartialFilter)
	}
}

func TestEnsureIndexWithRetryReturnsOnceTheIndexIsEnsured(t *testing.T) {
	fake := &fakeDBClient{results: []error{errors.New("no primary yet")}}
	withFakeDBClient(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spec := specNamed(t, amfUeSupiIndex().Name)
	if err := ensureIndexWithRetry(ctx, spec); err != nil {
		t.Fatalf("ensureIndexWithRetry gave up on an error that went away: %v", err)
	}
	if len(fake.calls) != 2 {
		t.Fatalf("expected a retry after the first failure, got %d call(s)", len(fake.calls))
	}
	if fake.calls[1].Name != spec.Name {
		t.Errorf("retried with spec %q, want %q", fake.calls[1].Name, spec.Name)
	}
}

func TestEnsureIndexWithRetryGivesUpWhenTheBudgetIsSpent(t *testing.T) {
	fake := &fakeDBClient{results: []error{errors.New("still no primary")}}
	withFakeDBClient(t, fake)

	// A budget already spent: the call must report the failure rather than
	// retry forever, because startup depends on hearing about it.
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	err := ensureIndexWithRetry(ctx, specNamed(t, amfUeSupiIndex().Name))
	if err == nil {
		t.Fatal("expected an error once the budget was spent")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error does not carry the deadline that caused it: %v", err)
	}
}

// TestEnsureIndexesEnsuresEveryOne covers the startup path itself rather
// than the specification: every index in amfUeIndexes has to reach the
// datastore, not merely be declared.
func TestEnsureIndexesEnsuresEveryOne(t *testing.T) {
	fake := &fakeDBClient{}
	withFakeDBClient(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), indexEnsureBudget)
	defer cancel()
	if err := ensureIndexes(ctx, amfUeIndexes()); err != nil {
		t.Fatalf("ensureIndexes failed: %v", err)
	}

	wanted := amfUeIndexes()
	if len(fake.calls) != len(wanted) {
		t.Fatalf("ensured %d index(es), want %d", len(fake.calls), len(wanted))
	}
	ensured := map[string]bool{}
	for _, call := range fake.calls {
		ensured[call.Name] = true
	}
	for _, spec := range wanted {
		if !ensured[spec.Name] {
			t.Errorf("index %q was declared but never ensured", spec.Name)
		}
	}
}

// TestEnsureIndexesReportsTheIndexItCouldNotEnsure covers the failure path
// rather than the exit. It is testable at all only because the failure is
// returned: ending the process here, as an earlier revision did, would take the
// test binary with it.
func TestEnsureIndexesReportsTheIndexItCouldNotEnsure(t *testing.T) {
	// One refusal per attempt, for longer than the budget allows attempts.
	refusals := make([]error, 64)
	for i := range refusals {
		refusals[i] = errors.New("not authorised to create indexes")
	}
	fake := &fakeDBClient{results: refusals}
	withFakeDBClient(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	err := ensureIndexes(ctx, amfUeIndexes())
	if err == nil {
		t.Fatal("expected the failure to be reported, not swallowed")
	}
	first := amfUeIndexes()[0]
	if !strings.Contains(err.Error(), first.Name) {
		t.Errorf("error does not name the index that failed (%q): %v", first.Name, err)
	}
	if !strings.Contains(err.Error(), "not authorised") {
		t.Errorf("error does not carry the cause that stopped it: %v", err)
	}
}

// TestEveryIndexIsEnsuredBeforeTheWriters pins the order the startup rests on:
// every index exists before any write is admitted.
//
// It drives prepareCollection, which is the function that decides the order,
// and watches when the writers are started relative to the EnsureIndex calls.
// An earlier version of this test called ensureIndexes directly and asserted
// only that each index was ensured -- which it would have done just as happily
// with the writers started first, the very case that can leave a unique index
// permanently uncreatable.
func TestEveryIndexIsEnsuredBeforeTheWriters(t *testing.T) {
	fake := &fakeDBClient{}
	withFakeDBClient(t, fake)

	writersStarted := false
	ensuredBeforeWriters := map[string]bool{}
	fake.onEnsure = func(spec mongoapi.IndexSpec) {
		if !writersStarted {
			ensuredBeforeWriters[spec.Name] = true
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), indexEnsureBudget)
	defer cancel()

	specs := amfUeIndexes()
	if err := prepareCollection(ctx, specs, func() { writersStarted = true }); err != nil {
		t.Fatalf("prepareCollection failed: %v", err)
	}

	if !writersStarted {
		t.Fatal("the writers were never started, so the order proves nothing")
	}
	for _, spec := range specs {
		if !ensuredBeforeWriters[spec.Name] {
			t.Errorf("index %q was not ensured before the write workers started: a concurrent upsert "+
				"can then make it permanently uncreatable", spec.Name)
		}
	}
}

// TestPrepareCollectionDoesNotStartWritersWhenAnIndexFails covers the other
// half of the same order: a failure must not leave writers running against a
// collection that never got its indexes.
func TestPrepareCollectionDoesNotStartWritersWhenAnIndexFails(t *testing.T) {
	refusals := make([]error, 64)
	for i := range refusals {
		refusals[i] = errors.New("not authorised to create indexes")
	}
	fake := &fakeDBClient{results: refusals}
	withFakeDBClient(t, fake)

	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	time.Sleep(time.Millisecond)

	writersStarted := false
	err := prepareCollection(ctx, amfUeIndexes(), func() { writersStarted = true })
	if err == nil {
		t.Fatal("expected the failure to be reported")
	}
	if writersStarted {
		t.Error("the write workers were started even though an index could not be ensured")
	}
}

// TestAmfUeSupiIndexIsTheOneEnsuredBeforeTheWriters pins the ordering that
// keeps one document per UE.
//
// Two workers upserting the same supi concurrently can each insert while no
// unique index exists, and a collection that has acquired such a pair can never
// have the index created again -- not on this start and not on any later one,
// which for an AMF that refuses to serve without it means it never starts
// again. Sparse does not rescue it either: the duplicate is a present value,
// not an absent one.
func TestAmfUeSupiIndexIsTheOneEnsuredBeforeTheWriters(t *testing.T) {
	supi := amfUeSupiIndex()
	if !supi.Unique {
		t.Error("the supi index is what keeps one document per UE; without Unique the ordering it is ensured in buys nothing")
	}
	if len(supi.Keys) != 1 || supi.Keys[0].Key != supiField {
		t.Errorf("the index ensured before the writers is over %v, not supi, which is the field every write upserts on", supi.Keys)
	}

	// And it must still be one of the indexes the collection ends up with.
	found := false
	for _, spec := range amfUeIndexes() {
		if spec.Name == supi.Name {
			found = true
			if spec.Unique != supi.Unique || spec.Sparse != supi.Sparse ||
				len(spec.Keys) != len(supi.Keys) || spec.Keys[0].Key != supi.Keys[0].Key {
				t.Errorf("amfUeIndexes carries a different %q from the one ensured first: %+v vs %+v",
					supi.Name, spec, supi)
			}
		}
	}
	if !found {
		t.Errorf("index %q is ensured before the writers but is not in amfUeIndexes", supi.Name)
	}
}

// TestAmfUeIndexesOverOmitemptyFieldsAreSparse covers the failure that cannot be
// recovered from: a unique index over a field some document lacks is refused
// once two such documents exist, and refused on every later start too.
func TestAmfUeIndexesOverOmitemptyFieldsAreSparse(t *testing.T) {
	// guti and tmsi are `omitempty` in AmfUe and are written only once the
	// value is assigned, so a context stored before that carries no such field.
	//
	// supi is `omitempty` too and is deliberately not here: every write is an
	// upsert filtered on it, and MongoDB copies a filter's equality fields into
	// any document it inserts, so no document this code writes can lack it.
	// See amfUeSupiIndex for why that difference is worth keeping.
	for _, name := range []string{"amfUeByGuti", "amfUeByTmsi"} {
		spec := specNamed(t, name)
		if spec.Unique && !spec.Sparse {
			t.Errorf("%s is unique over an omitempty field without Sparse: two documents lacking it "+
				"make the index permanently uncreatable", name)
		}
	}
}

// TestAmfUeSupiIndexMatchesTheOneAlreadyDeployed keeps the identity index a
// no-op to ensure rather than a replacement.
//
// Several AMF instances share one collection, and nothing here can stop the
// others writing while this one replaces an index. Dropping and recreating the
// index over supi is the dangerous case, because it is what stops two
// concurrent upserts making two documents for one UE -- so its specification
// has to stay exactly what the CreateIndex call this change replaces produced:
// the key alone, unique, under MongoDB's own generated name for that key.
func TestAmfUeSupiIndexMatchesTheOneAlreadyDeployed(t *testing.T) {
	supi := amfUeSupiIndex()

	if supi.Name != "supi_1" {
		t.Errorf("supi index is named %q; CreateIndex named it \"supi_1\", and a different name makes "+
			"ensuring it a drop and recreate while other instances are writing", supi.Name)
	}
	if !supi.Unique {
		t.Error("supi index is not unique, which is the property the deployed one has")
	}
	if supi.Sparse {
		t.Error("supi index is sparse; the deployed one is not, so this turns a no-op into a replacement")
	}
	if supi.PartialFilter != nil {
		t.Errorf("supi index carries a partial filter (%v); the deployed one has none", supi.PartialFilter)
	}
}

// TestRedactedMongoURLDropsEverySecretAConnectionStringCanCarry covers the log
// line that is the only record of which datastore the AMF used, and would
// otherwise print whatever the connection string holds -- in the userinfo
// password and in the query, which are two different places.
func TestRedactedMongoURLDropsEverySecretAConnectionStringCanCarry(t *testing.T) {
	// The host and database are all this log line is for, so every query case
	// reduces to the same thing.
	const hostAndDB = "mongodb://host/db"
	const unparseable = "(unparseable connection string)"

	cases := map[string]string{
		"mongodb://mongodb:27017":                      "mongodb://mongodb:27017",
		"mongodb://user:hunter2@host:27017/db":         "mongodb://user:xxxxx@host:27017/db",
		"mongodb+srv://admin:s3cr3t@cluster/?tls=true": "mongodb+srv://admin:xxxxx@cluster/",
		// The query carries secrets of its own, which url.Redacted does not
		// touch: these are real MongoDB options, matched case-insensitively by
		// the driver.
		hostAndDB + "?tlsCertificateKeyFilePassword=keypass":         hostAndDB,
		hostAndDB + "?sslClientCertificateKeyPassword=keypass":       hostAndDB,
		hostAndDB + "?authMechanismProperties=AWS_SESSION_TOKEN:tok": hostAndDB,
		hostAndDB + "?replicaSet=rs0&tls=true":                       hostAndDB,
		// A username is not a secret and is worth keeping; a password never
		// survives, whether or not one is present.
		"mongodb://user@host/db": "mongodb://user@host/db",
		// No authority at all. url.Parse takes these happily -- "user" becomes
		// the scheme and the rest is opaque -- and Redacted hands them back
		// whole, password and all, because there is no User for it to mask.
		"user:opaquepw@host": unparseable,
		"host:27017":         unparseable,
		"://bad":             unparseable,
	}
	secrets := []string{"hunter2", "s3cr3t", "keypass", "tok", "opaquepw"}
	for raw, want := range cases {
		got := redactedMongoURL(raw)
		if got != want {
			t.Errorf("redactedMongoURL(%q) = %q, want %q", raw, got, want)
		}
		for _, secret := range secrets {
			if strings.Contains(got, secret) {
				t.Errorf("redactedMongoURL(%q) leaked %q: %q", raw, secret, got)
			}
		}
	}
}
