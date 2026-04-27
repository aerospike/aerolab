package db

import (
	"context"
	"fmt"
	"sort"
	"testing"
)

// TestQueryIndexRangeMirrorsPluginTimeseries simulates the hot path: range on
// timestamp (indexed) + Eq on int label + BinExists on value column, with
// explicit projection.
func TestQueryIndexRangeMirrorsPluginTimeseries(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("latencies", []ColumnSpec{
		{Name: "timestamp", Type: TypeInt64, Indexed: true},
		{Name: "ClusterName", Type: TypeInt64},
		{Name: "value", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}
	// 100 rows across 10 clusters.
	for i := 0; i < 100; i++ {
		cluster := int64(i % 10)
		_ = d.Put("latencies", fmt.Sprintf("r%03d", i), Row{
			"timestamp":   Int(int64(1000 + i)),
			"ClusterName": Int(cluster),
			"value":       Int(int64(i * 2)),
		})
	}

	ctx := context.Background()
	iter := d.Query("latencies").
		Between("timestamp", Int(1010), Int(1050)).
		Where(And(Eq("ClusterName", Int(3)), Exists("value"))).
		Project("timestamp", "value").
		Run(ctx)
	defer iter.Close()

	got := 0
	seenTS := map[int64]bool{}
	for iter.Next() {
		_, row := iter.Record()
		if _, ok := row["ClusterName"]; ok {
			t.Error("ClusterName should not be projected")
		}
		ts, okT := row["timestamp"].AsInt()
		_, okV := row["value"].AsInt()
		if !okT || !okV {
			t.Errorf("expected timestamp + value, got row=%v", row)
			continue
		}
		if ts < 1010 || ts > 1050 {
			t.Errorf("timestamp %d out of range", ts)
		}
		seenTS[ts] = true
		got++
	}
	if iter.Err() != nil {
		t.Fatal(iter.Err())
	}
	// In range [1010,1050] inclusive, cluster==3 matches indexes i where
	// i%10==3: i in {13, 23, 33, 43}. All have value column.
	wantTS := []int64{1013, 1023, 1033, 1043}
	if got != len(wantTS) {
		t.Errorf("matched %d rows, want %d", got, len(wantTS))
	}
	for _, ts := range wantTS {
		if !seenTS[ts] {
			t.Errorf("expected timestamp %d was not returned", ts)
		}
	}
}

// TestQueryIndexRangeSortedOrder verifies the index yields rows in timestamp
// order.
func TestQueryIndexRangeSortedOrder(t *testing.T) {
	d := openTestDB(t)
	_ = d.RegisterSet("m", []ColumnSpec{{Name: "t", Type: TypeInt64, Indexed: true}})
	// Insert in shuffled order.
	for _, ts := range []int64{500, 100, 400, 200, 300} {
		_ = d.Put("m", fmt.Sprintf("k%d", ts), Row{"t": Int(ts)})
	}
	it := d.Query("m").Between("t", Int(0), Int(1000)).Run(context.Background())
	defer it.Close()
	var got []int64
	for it.Next() {
		_, r := it.Record()
		v, _ := r["t"].AsInt()
		got = append(got, v)
	}
	want := []int64{100, 200, 300, 400, 500}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	if !sort.SliceIsSorted(got, func(i, j int) bool { return got[i] < got[j] }) {
		t.Errorf("expected sorted, got %v", got)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("pos %d: got %d want %d", i, got[i], want[i])
		}
	}
}

// TestQueryTableShape mimics the plugin table path: Query with no range,
// filter by string equality, no projection.
func TestQueryTableShape(t *testing.T) {
	d := openTestDB(t)
	_ = d.Put("tbl", "r1", Row{"svc": Str("auth"), "n": Int(1)})
	_ = d.Put("tbl", "r2", Row{"svc": Str("cart"), "n": Int(2)})
	_ = d.Put("tbl", "r3", Row{"svc": Str("auth"), "n": Int(3)})

	it := d.Query("tbl").Where(Eq("svc", Str("auth"))).Run(context.Background())
	defer it.Close()
	got := map[string]int64{}
	for it.Next() {
		k, r := it.Record()
		n, _ := r["n"].AsInt()
		got[k] = n
	}
	if len(got) != 2 || got["r1"] != 1 || got["r3"] != 3 {
		t.Errorf("unexpected result: %v", got)
	}
}

// TestQueryHistogramShape mimics frontendHandleHistogram: range on timestamp
// + Eq(int) metric id + Eq(int) cluster id; projection over fixed bucket
// column names.
func TestQueryHistogramShape(t *testing.T) {
	d := openTestDB(t)
	_ = d.RegisterSet("hist", []ColumnSpec{
		{Name: "timestamp", Type: TypeInt64, Indexed: true},
		{Name: "metric_idx", Type: TypeInt64},
		{Name: "ClusterName", Type: TypeInt64},
	})
	// Insert some histogram rows. Bucket bins "00".."03" for simplicity.
	for i := 0; i < 30; i++ {
		metric := int64(i % 3)
		cluster := int64(i % 2)
		_ = d.Put("hist", fmt.Sprintf("h%d", i), Row{
			"timestamp":   Int(int64(10000 + i)),
			"metric_idx":  Int(metric),
			"ClusterName": Int(cluster),
			"00":          Int(int64(i)),
			"01":          Int(int64(i * 2)),
			"02":          Int(int64(i * 3)),
			"03":          Int(int64(i * 4)),
		})
	}

	it := d.Query("hist").
		Between("timestamp", Int(10000), Int(10020)).
		Where(And(Eq("metric_idx", Int(1)), Eq("ClusterName", Int(1)))).
		Project("00", "01", "02", "03").
		Run(context.Background())
	defer it.Close()

	matches := 0
	for it.Next() {
		_, row := it.Record()
		if _, ok := row["timestamp"]; ok {
			t.Error("timestamp should not be projected")
		}
		if _, ok := row["metric_idx"]; ok {
			t.Error("metric_idx should not be projected")
		}
		for _, b := range []string{"00", "01", "02", "03"} {
			if _, ok := row[b].AsInt(); !ok {
				t.Errorf("bucket %s missing", b)
			}
		}
		matches++
	}
	if it.Err() != nil {
		t.Fatal(it.Err())
	}
	// metric==1 && cluster==1 && 10000<=ts<=10020 => i in {1, 7, 13, 19}.
	if matches != 4 {
		t.Errorf("got %d matches, want 4", matches)
	}
}

func TestQueryBetweenOnUnindexedFallbackScan(t *testing.T) {
	d := openTestDB(t)
	// No indexed column; Between falls through to full-set scan.
	for i := 0; i < 5; i++ {
		_ = d.Put("noidx", fmt.Sprintf("k%d", i), Row{"v": Int(int64(i))})
	}
	it := d.Query("noidx").Between("v", Int(2), Int(3)).Run(context.Background())
	defer it.Close()
	var got []int64
	for it.Next() {
		_, r := it.Record()
		v, _ := r["v"].AsInt()
		got = append(got, v)
	}
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if len(got) != 2 || got[0] != 2 || got[1] != 3 {
		t.Errorf("expected [2,3], got %v", got)
	}
}

func TestQueryOrMustExistOptional(t *testing.T) {
	d := openTestDB(t)
	_ = d.Put("t", "r1", Row{"tag": Int(1), "label": Int(10)})
	_ = d.Put("t", "r2", Row{"tag": Int(2), "label": Int(20)})
	_ = d.Put("t", "r3", Row{"tag": Int(1)}) // no label
	// Plugin pattern for optional filter: Or(Not(Exists), Or(Eq values))
	filter := Or(Not(Exists("label")), Or(Eq("label", Int(20))))
	it := d.Query("t").Where(And(Eq("tag", Int(1)), filter)).Run(context.Background())
	defer it.Close()
	seen := map[string]bool{}
	for it.Next() {
		k, _ := it.Record()
		seen[k] = true
	}
	if !seen["r1"] && !seen["r3"] {
		// r1.tag=1, label=10; filter: not(exists)||Eq(20) => Eq(10,20) false, Exists=true, Not=false => fails
		// So r1 should NOT match.
	}
	if seen["r1"] {
		t.Error("r1 should not match (label=10, not 20, and Exists=true)")
	}
	if !seen["r3"] {
		t.Error("r3 should match via Not(Exists) branch")
	}
	if seen["r2"] {
		t.Error("r2 should not match (tag=2)")
	}
}

func TestQueryContextCancel(t *testing.T) {
	d := openTestDB(t)
	for i := 0; i < 1000; i++ {
		_ = d.Put("big", fmt.Sprintf("k%04d", i), Row{"v": Int(int64(i))})
	}
	ctx, cancel := context.WithCancel(context.Background())
	it := d.Query("big").Run(ctx)
	defer it.Close()
	if !it.Next() {
		t.Fatal("first Next should succeed")
	}
	cancel()
	// Drain; at some point Next should return false and Err should be set.
	saw := 0
	for it.Next() {
		saw++
		if saw > 10000 {
			t.Fatal("iterator did not stop after cancel")
		}
	}
	if it.Err() != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", it.Err())
	}
}

func TestQueryEmptySetReturnsEmpty(t *testing.T) {
	d := openTestDB(t)
	it := d.Query("never_created").Run(context.Background())
	defer it.Close()
	if it.Next() {
		t.Error("expected no rows")
	}
	if it.Err() != nil {
		t.Error(it.Err())
	}
}

// TestQueryIndexSnapshotIsolation verifies that a Query on an indexed range
// observes a consistent point-in-time view: writes committed after Run()
// returned must not appear in the in-flight iteration. This models plugin
// range scans running against a database that ingest is actively writing to.
func TestQueryIndexSnapshotIsolation(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("m", []ColumnSpec{{Name: "ts", Type: TypeInt64, Indexed: true}}); err != nil {
		t.Fatal(err)
	}
	for i := int64(1); i <= 200; i++ {
		if err := d.Put("m", fmt.Sprintf("k%04d", i), Row{"ts": Int(i)}); err != nil {
			t.Fatal(err)
		}
	}

	it := d.Query("m").Between("ts", Int(1), Int(1_000_000)).Run(context.Background())
	defer it.Close()
	// Consume the first record so the iterator has been positioned; from
	// this point the snapshot is pinned.
	if !it.Next() {
		t.Fatal("expected at least one record")
	}

	// Write new rows with timestamps in the query range. They must NOT
	// leak into the in-flight iteration.
	for i := int64(201); i <= 400; i++ {
		if err := d.Put("m", fmt.Sprintf("k%04d", i), Row{"ts": Int(i)}); err != nil {
			t.Fatal(err)
		}
	}

	seen := 1 // the first we already consumed
	for it.Next() {
		_, r := it.Record()
		ts, _ := r["ts"].AsInt()
		if ts > 200 {
			t.Errorf("snapshot leaked new write ts=%d into iterator", ts)
		}
		seen++
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 200 {
		t.Errorf("expected 200 records in snapshot, saw %d", seen)
	}
}

// TestQueryFullScanSnapshotIsolation verifies the same property for the
// non-indexed (full set scan) path that Where(...) without Between uses.
func TestQueryFullScanSnapshotIsolation(t *testing.T) {
	d := openTestDB(t)
	for i := int64(1); i <= 200; i++ {
		if err := d.Put("svc", fmt.Sprintf("k%04d", i), Row{"tag": Int(1), "n": Int(i)}); err != nil {
			t.Fatal(err)
		}
	}
	it := d.Query("svc").Where(Eq("tag", Int(1))).Run(context.Background())
	defer it.Close()
	if !it.Next() {
		t.Fatal("expected at least one record")
	}
	for i := int64(201); i <= 400; i++ {
		if err := d.Put("svc", fmt.Sprintf("k%04d", i), Row{"tag": Int(1), "n": Int(i)}); err != nil {
			t.Fatal(err)
		}
	}
	seen := 1
	for it.Next() {
		_, r := it.Record()
		n, _ := r["n"].AsInt()
		if n > 200 {
			t.Errorf("snapshot leaked new write n=%d into full scan", n)
		}
		seen++
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 200 {
		t.Errorf("expected 200 records in snapshot, saw %d", seen)
	}
}

// TestScanSnapshotIsolation verifies Scan (used by plugin's labels cache
// refresh) is isolated from concurrent writes.
func TestScanSnapshotIsolation(t *testing.T) {
	d := openTestDB(t)
	for i := int64(1); i <= 100; i++ {
		if err := d.Put("labels", fmt.Sprintf("lbl%03d", i), Row{"v": Int(i)}); err != nil {
			t.Fatal(err)
		}
	}
	it := d.Scan("labels")
	defer it.Close()
	if !it.Next() {
		t.Fatal("expected at least one record")
	}
	for i := int64(101); i <= 200; i++ {
		if err := d.Put("labels", fmt.Sprintf("lbl%03d", i), Row{"v": Int(i)}); err != nil {
			t.Fatal(err)
		}
	}
	seen := 1
	for it.Next() {
		_, r := it.Record()
		v, _ := r["v"].AsInt()
		if v > 100 {
			t.Errorf("snapshot leaked new write v=%d into scan", v)
		}
		seen++
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 100 {
		t.Errorf("expected 100 records in snapshot, saw %d", seen)
	}
}

func TestSchemaOfAndSetsReflectState(t *testing.T) {
	d := openTestDB(t)
	_ = d.RegisterSet("x", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "v", Type: TypeString},
	})
	_ = d.Put("y", "k", Row{"n": Int(1)})
	sets := d.Sets()
	sort.Strings(sets)
	if len(sets) != 2 || sets[0] != "x" || sets[1] != "y" {
		t.Errorf("sets: %v", sets)
	}
	schema, ok := d.SchemaOf("x")
	if !ok {
		t.Fatal("schema of x missing")
	}
	var indexedCount int
	seen := map[string]ColumnType{}
	for _, c := range schema {
		seen[c.Name] = c.Type
		if c.Indexed {
			indexedCount++
		}
	}
	if indexedCount != 1 {
		t.Errorf("want 1 indexed col, got %d", indexedCount)
	}
	if seen["ts"] != TypeInt64 || seen["v"] != TypeString {
		t.Errorf("schema mismatch: %+v", seen)
	}
}

// TestIndexedScanIsSingleIterator asserts the v2 clustered-by-time read
// path: an indexed Between() walking N rows must perform zero per-row
// Pebble point Gets — the entire row payload is read out of the
// iterator's Value buffer.
func TestIndexedScanIsSingleIterator(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("ts", []ColumnSpec{
		{Name: "timestamp", Type: TypeInt64, Indexed: true},
		{Name: "ClusterName", Type: TypeInt64},
		{Name: "value", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}
	const rows = 1000
	for i := 0; i < rows; i++ {
		if err := d.Put("ts", fmt.Sprintf("r%05d", i), Row{
			"timestamp":   Int(int64(1_000_000 + i)),
			"ClusterName": Int(int64(i % 7)),
			"value":       Int(int64(i)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	before := d.Stats().PebbleGets
	ctx := context.Background()
	it := d.Query("ts").
		Between("timestamp", Int(1_000_100), Int(1_000_900)).
		Project("timestamp", "value").
		Run(ctx)
	defer it.Close()
	seen := 0
	for it.Next() {
		seen++
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 801 {
		t.Errorf("want 801 rows, got %d", seen)
	}
	after := d.Stats().PebbleGets
	if delta := after - before; delta != 0 {
		t.Errorf("indexed Between performed %d Pebble Gets; expected 0 (single-iterator path)", delta)
	}
}

// TestIndexedScanZeroGetsAfterAssumeNew is the regression guard for
// the orphan-guard auto-flip we removed: an AGI-style ingest workload
// uses AssumeNew=true on every Put, but with IndexCanHaveOrphans=false
// (the default) the read path MUST remain at zero per-row Pebble Gets.
// Before this fix the same scan issued one Get per row, costing a
// 4-5× slowdown vs Aerospike on the user's 2.27 M-row histogram set.
func TestIndexedScanZeroGetsAfterAssumeNew(t *testing.T) {
	d := openTestDB(t)
	if d.opts.IndexCanHaveOrphans {
		t.Fatal("openTestDB unexpectedly enabled IndexCanHaveOrphans")
	}
	if err := d.RegisterSet("ts", []ColumnSpec{
		{Name: "timestamp", Type: TypeInt64, Indexed: true},
		{Name: "v", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}
	const rows = 1000
	items := make([]PutItem, rows)
	for i := 0; i < rows; i++ {
		items[i] = PutItem{
			Key:       fmt.Sprintf("r%05d", i),
			Row:       Row{"timestamp": Int(int64(1_000_000 + i)), "v": Int(int64(i))},
			AssumeNew: true,
		}
	}
	if err := d.PutBatch("ts", items); err != nil {
		t.Fatal(err)
	}
	if d.assumeNewSeen.Load() {
		t.Fatal("assumeNewSeen flipped on its own; the auto-flip was supposed to be removed")
	}
	before := d.Stats().PebbleGets
	ctx := context.Background()
	it := d.Query("ts").
		Between("timestamp", Int(1_000_100), Int(1_000_900)).
		Run(ctx)
	defer it.Close()
	seen := 0
	for it.Next() {
		seen++
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	if seen != 801 {
		t.Errorf("want 801 rows, got %d", seen)
	}
	after := d.Stats().PebbleGets
	if delta := after - before; delta != 0 {
		t.Errorf("post-AssumeNew indexed Between performed %d Pebble Gets; expected 0 (orphan-guard must stay OFF by default)", delta)
	}
}

// TestPlanClassifiesExistsAsPresenceOnly asserts that buildPlan
// partitions filter columns so that Exists / Not(Exists) references
// land in the presence bitmap and never trigger decodePayload, while
// any value-using predicate (Eq/In/Between) on the same row promotes
// the column into the value-decode set even if it also appears under
// an Exists.
//
// Shape mirrors the AGI histogram dashboard: indexed Between on
// timestamp + 3 BinExists + 1 Or(Not(Exists), Eq) on a 4th column.
// Histogram, ClusterName, NodeIdent must be presence-only; Namespace
// must be value-needed because of the inner Eq.
func TestPlanClassifiesExistsAsPresenceOnly(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("agi", []ColumnSpec{
		{Name: "timestamp", Type: TypeInt64, Indexed: true},
		{Name: "Histogram", Type: TypeString},
		{Name: "ClusterName", Type: TypeInt64},
		{Name: "NodeIdent", Type: TypeString},
		{Name: "Namespace", Type: TypeString},
		{Name: "value", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}
	s, ok := d.lookupSet("agi")
	if !ok {
		t.Fatal("set not registered")
	}
	bt := &betweenClause{col: "timestamp", lo: Int(0), hi: Int(1)}
	filter := And(
		Exists("Histogram"),
		Exists("ClusterName"),
		Exists("NodeIdent"),
		Or(Not(Exists("Namespace")), Eq("Namespace", Str("ns0"))),
	)
	s.mu.RLock()
	plan := buildPlan(s, []string{"timestamp", "value"}, filter, bt, true)
	histID := s.Columns["Histogram"].ID
	clusterID := s.Columns["ClusterName"].ID
	nodeID := s.Columns["NodeIdent"].ID
	nsID := s.Columns["Namespace"].ID
	tsID := s.Columns["timestamp"].ID
	valID := s.Columns["value"].ID
	s.mu.RUnlock()

	wantPresence := map[uint32]bool{histID: true, clusterID: true, nodeID: true}
	for id := range wantPresence {
		if !bitmapTest(plan.mask.wantPresenceBM, id) {
			t.Errorf("colID %d should be in wantPresenceBM (presence-only)", id)
		}
		if bitmapTest(plan.mask.wantValueBM, id) {
			t.Errorf("colID %d should NOT be in wantValueBM (presence-only)", id)
		}
	}
	if !bitmapTest(plan.mask.wantValueBM, nsID) {
		t.Error("Namespace must be value-needed (inner Eq)")
	}
	if bitmapTest(plan.mask.wantPresenceBM, nsID) {
		t.Error("Namespace must NOT be in presence set (value supersedes)")
	}
	if !bitmapTest(plan.mask.wantValueBM, tsID) {
		t.Error("projected timestamp must be value-needed")
	}
	if !bitmapTest(plan.mask.wantValueBM, valID) {
		t.Error("projected value must be value-needed")
	}
	if plan.mask.decodeAll {
		t.Error("explicit projection should not produce decodeAll mask")
	}
}

// TestPlanNoProjectionUsesDecodeAll asserts the no-projection path
// keeps the existing "decode every column" semantics: the codec walks
// the entire row and the planner's classification is irrelevant for
// the codec (it stays relevant only for documentation / future
// optimization).
func TestPlanNoProjectionUsesDecodeAll(t *testing.T) {
	d := openTestDB(t)
	if err := d.RegisterSet("m", []ColumnSpec{
		{Name: "ts", Type: TypeInt64, Indexed: true},
		{Name: "c", Type: TypeInt64},
	}); err != nil {
		t.Fatal(err)
	}
	s, _ := d.lookupSet("m")
	s.mu.RLock()
	plan := buildPlan(s, nil, Exists("c"), nil, false)
	s.mu.RUnlock()
	if !plan.mask.decodeAll {
		t.Error("no-projection plan must produce decodeAll mask")
	}
	if plan.projectionExplicit {
		t.Error("projectionExplicit must be false")
	}
}
