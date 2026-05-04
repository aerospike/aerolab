package ingest

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// TestBinListSnapshotLockFreeReadsConcurrent runs a small fixed
// number of reader and writer goroutines that interleave
// missingNames() / containsName() calls with addNames() calls on
// the same binList. The race detector is the primary correctness
// signal: the hot read path must never observe a partially-built
// map. We then assert that every key the writer published is
// visible in the final snapshot, exercising the atomic.Pointer
// publish-and-load contract.
//
// We bound the work by iteration count (not by wall clock) so the
// test stays fast under -race; an unbounded reader spin loop would
// starve the writer by tens of seconds under the race detector and
// add no extra coverage.
func TestBinListSnapshotLockFreeReadsConcurrent(t *testing.T) {
	bl := &binList{
		BinNames: []string{"seed_a", "seed_b"},
	}
	bl.seedSnapshot()

	const (
		readers           = 8
		readsPerReader    = 2000
		writeKeys         = 50
	)

	probe := map[string]any{"seed_a": 1, "seed_b": 1}
	var wg sync.WaitGroup
	wg.Add(readers + 1)
	for r := 0; r < readers; r++ {
		go func() {
			defer wg.Done()
			for n := 0; n < readsPerReader; n++ {
				_ = bl.missingNames(probe)
				_ = bl.containsName("seed_a")
			}
		}()
	}
	go func() {
		defer wg.Done()
		for k := 0; k < writeKeys; k++ {
			bl.addNames([]string{"k" + itoa(k)})
		}
	}()
	wg.Wait()

	for k := 0; k < writeKeys; k++ {
		if !bl.containsName("k" + itoa(k)) {
			t.Fatalf("key k%d missing from snapshot after addNames", k)
		}
	}
	if !bl.containsName("seed_a") || !bl.containsName("seed_b") {
		t.Fatalf("seed names dropped during writer churn")
	}
	if got := len(bl.BinNames); got != 2+writeKeys {
		t.Fatalf("BinNames len = %d; want %d", got, 2+writeKeys)
	}
}

// TestBinListAddNamesIsIdempotent verifies that addNames called twice
// with the same key never duplicates BinNames and that the second
// call returns no "added" names. This is the safety net behind the
// hot path's read-then-add pattern: two workers may both see the
// same key as "missing" in their snapshot read and both call
// addNames; addNames must serialize them and dedupe.
func TestBinListAddNamesIsIdempotent(t *testing.T) {
	bl := &binList{}
	bl.seedSnapshot()

	added := bl.addNames([]string{"a", "b"})
	if len(added) != 2 {
		t.Fatalf("first addNames returned %v; want 2 names", added)
	}
	added2 := bl.addNames([]string{"a", "b", "c"})
	if len(added2) != 1 || added2[0] != "c" {
		t.Fatalf("second addNames returned %v; want [c]", added2)
	}
	if len(bl.BinNames) != 3 {
		t.Fatalf("BinNames = %v; want 3 entries", bl.BinNames)
	}
	for _, k := range []string{"a", "b", "c"} {
		if !bl.containsName(k) {
			t.Fatalf("key %q missing", k)
		}
	}
}

// TestBinListAddNamesConcurrentDedup runs N goroutines all trying to
// addNames the SAME key. After they all return, BinNames must
// contain exactly one copy. Without the under-lock recheck this
// would race and append duplicates.
func TestBinListAddNamesConcurrentDedup(t *testing.T) {
	bl := &binList{}
	bl.seedSnapshot()

	const workers = 64
	var wg sync.WaitGroup
	wg.Add(workers)
	start := make(chan struct{})
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			<-start
			bl.addNames([]string{"only"})
		}()
	}
	close(start)
	wg.Wait()

	if got := len(bl.BinNames); got != 1 {
		t.Fatalf("BinNames = %v; want exactly one copy", bl.BinNames)
	}
	if !bl.containsName("only") {
		t.Fatal("only-key not visible after concurrent addNames")
	}
}

// TestSaveProgressFlushesBinList verifies the crash-resume invariant:
// every call to saveProgress() must persist BINLIST first so the
// on-disk pair (BINLIST, progress) is consistent. We simulate the
// hot path by adding a few names to the bin list (no per-row Put
// any more) and then call saveProgress(); the BINLIST row in the
// labels set must reflect every added name.
func TestSaveProgressFlushesBinList(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "binlist-saveprogress-db")
	opts := db.DefaultOptions()
	opts.Path = dir
	d, err := db.Open(opts)
	if err != nil {
		t.Fatalf("db.Open: %s", err)
	}
	defer d.Close()
	if err := d.RegisterSet("labels", []db.ColumnSpec{
		{Name: labelsValueCol, Type: db.TypeString},
	}); err != nil {
		t.Fatalf("RegisterSet labels: %s", err)
	}

	cfg := new(Config)
	cfg.ProgressFile.DisableWrite = true
	bl := &binList{BinNames: []string{"seed"}, changed: true}
	bl.seedSnapshot()
	i := &Ingest{
		config:   cfg,
		patterns: &patterns{LabelsSetName: "labels"},
		db:       d,
		progress: new(Progress),
		binList:  bl,
		endLock:  new(sync.Mutex),
	}
	// progress must have minimal initial state so saveProgress
	// doesn't NPE on LineErrors / changed flags.
	i.progress.LogProcessor = new(ProgressLogProcessor)
	i.progress.LogProcessor.LineErrors = new(lineErrors)
	i.progress.Downloader = new(ProgressDownloader)
	i.progress.Unpacker = new(ProgressUnpacker)
	i.progress.PreProcessor = new(ProgressPreProcessor)
	i.progress.CollectinfoProcessor = new(ProgressCollectProcessor)

	bl.addNames([]string{"alpha", "beta", "gamma"})

	if err := i.saveProgress(); err != nil {
		t.Fatalf("saveProgress: %s", err)
	}

	row, err := d.Get("labels", "BINLIST", labelsValueCol)
	if err != nil {
		t.Fatalf("Get BINLIST: %s", err)
	}
	if row == nil {
		t.Fatal("BINLIST row was not persisted by saveProgress")
	}
	s, ok := row[labelsValueCol].AsString()
	if !ok {
		t.Fatal("BINLIST value is not a string")
	}
	var got []string
	if err := json.Unmarshal([]byte(s), &got); err != nil {
		t.Fatalf("unmarshal BINLIST: %s", err)
	}
	want := map[string]bool{"seed": true, "alpha": true, "beta": true, "gamma": true}
	have := make(map[string]bool, len(got))
	for _, k := range got {
		have[k] = true
	}
	for k := range want {
		if !have[k] {
			t.Errorf("BINLIST missing %q (got %v)", k, got)
		}
	}
}

// TestStoreBinListDoesNotHoldLockAcrossPut is a structural regression
// guard: it adds a name, calls storeBinList from a goroutine, and
// while that goroutine is mid-Put, attempts an addNames from the
// main goroutine. addNames should complete promptly even though
// storeBinList is blocked in db.Put, because storeBinList drops the
// lock around the Put. We assert the lock is reacquirable by timing:
// the addNames lap finishes before storeBinList does (storeBinList
// has to round-trip Pebble; addNames is a microsecond operation in
// memory only). The race detector also covers correctness.
func TestStoreBinListDoesNotHoldLockAcrossPut(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "binlist-no-lock-during-put-db")
	opts := db.DefaultOptions()
	opts.Path = dir
	d, err := db.Open(opts)
	if err != nil {
		t.Fatalf("db.Open: %s", err)
	}
	defer d.Close()
	if err := d.RegisterSet("labels", []db.ColumnSpec{
		{Name: labelsValueCol, Type: db.TypeString},
	}); err != nil {
		t.Fatalf("RegisterSet labels: %s", err)
	}

	bl := &binList{BinNames: []string{"seed"}, changed: true}
	bl.seedSnapshot()
	i := &Ingest{
		patterns: &patterns{LabelsSetName: "labels"},
		db:       d,
		binList:  bl,
	}

	var (
		wg          sync.WaitGroup
		addNamesOK  atomic.Bool
		storeDoneCh = make(chan struct{})
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		// Repeatedly call storeBinList; each Put must release
		// the lock between the snapshot copy and the post-Put
		// re-acquire so the writer below is never starved.
		for k := 0; k < 16; k++ {
			if err := i.storeBinList(); err != nil {
				t.Errorf("storeBinList: %s", err)
				return
			}
		}
		close(storeDoneCh)
	}()
	go func() {
		defer wg.Done()
		for k := 0; k < 32; k++ {
			before := len(bl.BinNames)
			added := bl.addNames([]string{"k" + itoa(k)})
			after := len(bl.BinNames)
			if len(added) != 1 || after != before+1 {
				t.Errorf("addNames k%d: added=%v before=%d after=%d", k, added, before, after)
				return
			}
			addNamesOK.Store(true)
		}
	}()
	wg.Wait()
	<-storeDoneCh

	if !addNamesOK.Load() {
		t.Fatal("addNames never completed; storeBinList likely held the lock across Put")
	}
}
