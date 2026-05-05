package ingest

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// makeIngestForUpsertTests wires the smallest possible Ingest shell
// that upsertMetaEntry needs: a real db so the labels-set Put path
// exercises a real backend, a minimal patterns with LabelsSetName
// registered, and a binList. Returns the Ingest, the parent
// metaShards (so the test can inspect the meta map), and a teardown
// function.
func makeIngestForUpsertTests(t *testing.T) (*Ingest, *MetaShards, func()) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "ingest-db")
	opts := db.DefaultOptions()
	opts.Path = dir
	d, err := db.Open(opts)
	if err != nil {
		t.Fatalf("db.Open: %s", err)
	}
	if err := d.RegisterSet("labels", []db.ColumnSpec{
		{Name: labelsValueCol, Type: db.TypeString},
	}); err != nil {
		_ = d.Close()
		t.Fatalf("RegisterSet labels: %s", err)
	}
	cfg := new(Config)
	cfg.TimestampColumnName = "timestamp"
	p := &patterns{LabelsSetName: "labels"}
	bl := &binList{}
	bl.seedSnapshot()
	i := &Ingest{
		config:   cfg,
		patterns: p,
		db:       d,
		binList:  bl,
	}
	shards := &MetaShards{meta: make(map[string]*metaEntries)}
	teardown := func() {
		_ = d.Close()
	}
	return i, shards, teardown
}

// TestUpsertMetaEntryConcurrentFirstSight runs N goroutines that all
// attempt to first-sight the same (k, sv) pair concurrently. After
// they all return, meta[k].Entries must contain exactly one copy of
// sv, and entriesIdx must agree with the slice index. Without per-key
// locking (or with a buggy double-checked init) two workers could
// both observe present=false, both append sv, and leave the index
// map pointing at the second copy while the first is unreachable.
func TestUpsertMetaEntryConcurrentFirstSight(t *testing.T) {
	i, shards, teardown := makeIngestForUpsertTests(t)
	defer teardown()

	const workers = 64
	var wg sync.WaitGroup
	wg.Add(workers)
	idxs := make([]int, workers)
	oks := make([]bool, workers)
	start := make(chan struct{})
	for w := 0; w < workers; w++ {
		w := w
		go func() {
			defer wg.Done()
			<-start
			me := shards.getOrCreate("namespace")
			idx, ok := i.upsertMetaEntry(me, "namespace", "test", "clusterA", "fn")
			idxs[w] = idx
			oks[w] = ok
		}()
	}
	close(start)
	wg.Wait()

	me := shards.meta["namespace"]
	if me == nil {
		t.Fatal("metaEntries for 'namespace' not created")
	}
	if len(me.Entries) != 1 {
		t.Fatalf("Entries = %v; want exactly one copy", me.Entries)
	}
	if me.Entries[0] != "test" {
		t.Fatalf("Entries[0] = %q; want %q", me.Entries[0], "test")
	}
	if got, ok := me.entriesIdx["test"]; !ok || got != 0 {
		t.Fatalf("entriesIdx[test] = (%d, %v); want (0, true)", got, ok)
	}
	if cl := me.ByCluster["clusterA"]; len(cl) != 1 || cl[0] != 0 {
		t.Fatalf("ByCluster[clusterA] = %v; want [0]", cl)
	}
	if _, ok := me.byClusterSet["clusterA"][0]; !ok {
		t.Fatalf("byClusterSet[clusterA][0] missing")
	}
	for w, ok := range oks {
		if !ok {
			t.Errorf("worker %d returned persisted=false", w)
		}
		if idxs[w] != 0 {
			t.Errorf("worker %d got idx=%d; want 0", w, idxs[w])
		}
	}
}

// TestUpsertMetaEntryRollbackOnDBPutFailure forces the db.Put for the
// labels row to fail by closing the db handle BEFORE invoking
// upsertMetaEntry. The function must roll back both the slice and the
// index-map mutations and return persisted=false; subsequent reads of
// the metaEntries must show no leaked Entries / ByCluster / index
// state.
func TestUpsertMetaEntryRollbackOnDBPutFailure(t *testing.T) {
	i, shards, _ := makeIngestForUpsertTests(t)
	// Close the db handle. Subsequent Put calls must fail.
	if err := i.db.Close(); err != nil {
		t.Fatalf("db.Close: %s", err)
	}

	me := shards.getOrCreate("namespace")
	// Pre-state: no entries, empty maps.
	if len(me.Entries) != 0 || len(me.entriesIdx) != 0 {
		t.Fatalf("pre-state not empty: %v / %v", me.Entries, me.entriesIdx)
	}

	idx, persisted := i.upsertMetaEntry(me, "namespace", "test", "clusterA", "fn")
	if persisted {
		t.Fatalf("expected persisted=false on closed db; got idx=%d persisted=true", idx)
	}
	if len(me.Entries) != 0 {
		t.Fatalf("Entries leaked after rollback: %v", me.Entries)
	}
	if len(me.entriesIdx) != 0 {
		t.Fatalf("entriesIdx leaked after rollback: %v", me.entriesIdx)
	}
	if cl := me.ByCluster["clusterA"]; len(cl) != 0 {
		t.Fatalf("ByCluster[clusterA] leaked after rollback: %v", cl)
	}
	if cs := me.byClusterSet["clusterA"]; len(cs) != 0 {
		t.Fatalf("byClusterSet[clusterA] leaked after rollback: %v", cs)
	}
}

// TestUpsertMetaEntrySecondSightingNewCluster covers the mutAppendCl
// branch: an existing entry observed under a NEW cluster only appends
// to ByCluster[newCluster] (no new Entries row). Rollback on Put
// failure must shrink ByCluster + byClusterSet without touching
// Entries / entriesIdx.
func TestUpsertMetaEntrySecondSightingNewCluster(t *testing.T) {
	i, shards, teardown := makeIngestForUpsertTests(t)
	defer teardown()

	me := shards.getOrCreate("namespace")
	if _, ok := i.upsertMetaEntry(me, "namespace", "test", "clusterA", "fn"); !ok {
		t.Fatal("first upsert failed")
	}
	if _, ok := i.upsertMetaEntry(me, "namespace", "test", "clusterB", "fn"); !ok {
		t.Fatal("second upsert failed")
	}
	if len(me.Entries) != 1 {
		t.Fatalf("Entries = %v; want 1 row", me.Entries)
	}
	if cl := me.ByCluster["clusterA"]; len(cl) != 1 || cl[0] != 0 {
		t.Fatalf("ByCluster[clusterA] = %v; want [0]", cl)
	}
	if cl := me.ByCluster["clusterB"]; len(cl) != 1 || cl[0] != 0 {
		t.Fatalf("ByCluster[clusterB] = %v; want [0]", cl)
	}
	if _, ok := me.byClusterSet["clusterB"][0]; !ok {
		t.Fatal("byClusterSet[clusterB][0] missing")
	}
	// Idempotent: same (sv, cluster) again should not append.
	if _, ok := i.upsertMetaEntry(me, "namespace", "test", "clusterB", "fn"); !ok {
		t.Fatal("third upsert failed")
	}
	if cl := me.ByCluster["clusterB"]; len(cl) != 1 {
		t.Fatalf("ByCluster[clusterB] = %v; want still [0] (idempotent)", cl)
	}
}
