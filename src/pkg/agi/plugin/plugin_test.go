package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// seedTestData populates a db.DB with a handful of rows that exercise the
// three cache refreshers (sets, bin list, metadata) and the query handlers.
func seedTestData(t *testing.T, d *db.DB, labelsSet string) {
	t.Helper()

	// BINLIST row in the labels set
	binList := []string{"latency_p99", "latency_p50", "op_count"}
	blJSON, err := json.Marshal(binList)
	if err != nil {
		t.Fatalf("marshal binlist: %s", err)
	}
	if err := d.Put(labelsSet, "BINLIST", db.Row{"json": db.Str(string(blJSON))}); err != nil {
		t.Fatalf("put BINLIST: %s", err)
	}

	// ClusterName label row: one cluster at index 0
	cluster := &metaEntries{
		Entries:   []string{"cluster-a"},
		ByCluster: map[string][]int{"cluster-a": {0}},
	}
	clJSON, err := json.Marshal(cluster)
	if err != nil {
		t.Fatalf("marshal ClusterName meta: %s", err)
	}
	if err := d.Put(labelsSet, "ClusterName", db.Row{"json": db.Str(string(clJSON))}); err != nil {
		t.Fatalf("put ClusterName: %s", err)
	}

	// NodeIdent label row so variable lookups work
	node := &metaEntries{
		Entries:   []string{"node-1"},
		ByCluster: map[string][]int{"cluster-a": {0}},
	}
	ndJSON, err := json.Marshal(node)
	if err != nil {
		t.Fatalf("marshal NodeIdent meta: %s", err)
	}
	if err := d.Put(labelsSet, "NodeIdent", db.Row{"json": db.Str(string(ndJSON))}); err != nil {
		t.Fatalf("put NodeIdent: %s", err)
	}

	// metrics set with an indexed timestamp column so Between uses the
	// primary index path.
	if err := d.RegisterSet("metrics", []db.ColumnSpec{
		{Name: "timestamp", Type: db.TypeInt64, Indexed: true},
	}); err != nil {
		t.Fatalf("register metrics set: %s", err)
	}
	base := time.Now().Add(-1 * time.Hour).UnixMilli()
	for i := 0; i < 5; i++ {
		ts := base + int64(i)*60_000
		pk := fmt.Sprintf("cluster-a/node-1/%d", ts)
		if err := d.Put("metrics", pk, db.Row{
			"timestamp":   db.Int(ts),
			"ClusterName": db.Int(0),
			"NodeIdent":   db.Int(0),
			"latency_p99": db.Int(int64(100 + i)),
			"op_count":    db.Int(int64(10 + i)),
		}); err != nil {
			t.Fatalf("put metrics row %d: %s", i, err)
		}
	}
}

// newTestPlugin opens an embedded DB at a fresh tempdir, seeds it, and
// returns a Plugin ready to handle HTTP requests. It intentionally does not
// call Listen() — we drive the handlers directly via httptest.
func newTestPlugin(t *testing.T) *Plugin {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "agi-db")
	yamlConfig := fmt.Sprintf("db:\n  path: %q\n  enableWAL: false\nlabelsSetName: labels\ntimestampBinName: timestamp\n", dir)
	config, err := MakeConfigReader(true, strings.NewReader(yamlConfig), false)
	if err != nil {
		t.Fatalf("MakeConfigReader: %s", err)
	}
	// Keep noise low unless explicitly overridden.
	config.LogLevel = 4
	// Background cache refresher not needed; we refresh synchronously
	// below after seeding.
	config.CacheRefreshInterval = time.Hour

	p, err := Init(config)
	if err != nil {
		t.Fatalf("Init: %s", err)
	}
	t.Cleanup(p.Close)

	seedTestData(t, p.db, config.LabelsSetName)

	// Drive one cache refresh synchronously so the HTTP handlers see
	// seeded state immediately.
	if err := p.cacheSetList(); err != nil {
		t.Fatalf("cacheSetList: %s", err)
	}
	if err := p.cacheBinList(); err != nil {
		t.Fatalf("cacheBinList: %s", err)
	}
	if err := p.cacheMetadataList(); err != nil {
		t.Fatalf("cacheMetadataList: %s", err)
	}
	return p
}

func TestMetricsHandler(t *testing.T) {
	p := newTestPlugin(t)

	req := httptest.NewRequest(http.MethodPost, "/metrics", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	p.handleMetrics(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	var resp []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %s", err)
	}
	// Both "labels" and "metrics" sets should be present.
	seen := map[string]bool{}
	for _, r := range resp {
		if v, ok := r["value"].(string); ok {
			seen[v] = true
		}
	}
	if !seen["labels"] || !seen["metrics"] {
		t.Fatalf("expected labels and metrics sets in response, got %+v", resp)
	}
}

func TestVariableHandler(t *testing.T) {
	p := newTestPlugin(t)

	body := `{"payload":{"target":"ClusterName"},"range":{"from":"2000-01-01T00:00:00Z","to":"2100-01-01T00:00:00Z"}}`
	req := httptest.NewRequest(http.MethodPost, "/variable", strings.NewReader(body))
	rec := httptest.NewRecorder()
	p.handleVariable(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp []struct {
		Text  string `json:"__text"`
		Value string `json:"__value"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if len(resp) != 1 || resp[0].Value != "cluster-a" {
		t.Fatalf("expected single cluster-a entry, got %+v", resp)
	}
}

func TestQueryTimeseriesHandler(t *testing.T) {
	p := newTestPlugin(t)

	body := fmt.Sprintf(`{
		"range":{"from":"%s","to":"%s"},
		"intervalMs":60000,
		"maxDataPoints":1000,
		"targets":[{
			"refId":"A",
			"target":"metrics",
			"payload":{
				"type":"timeseries",
				"timestampBinName":"timestamp",
				"bins":[{"name":"latency_p99","required":true}]
			}
		}],
		"scopedVars":{}
	}`,
		time.Now().Add(-2*time.Hour).Format(time.RFC3339),
		time.Now().Format(time.RFC3339),
	)
	req := httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(body))
	rec := httptest.NewRecorder()
	p.handleQuery(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var resp []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if len(resp) == 0 {
		t.Fatalf("expected at least one series, got %+v", resp)
	}
	dps, _ := resp[0]["datapoints"].([]any)
	if len(dps) == 0 {
		t.Fatalf("expected datapoints in series, got %+v", resp[0])
	}
}

// roundTrip is a thin convenience that asserts a 200 from a handler.
// Unused helpers here trip the lint, so we wrap it in a use below.
func roundTrip(t *testing.T, handler http.HandlerFunc, method, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, body)
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec
}

func TestPingHandler(t *testing.T) {
	p := newTestPlugin(t)
	rec := roundTrip(t, p.handlePing, http.MethodGet, "/", nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "OK" {
		t.Fatalf("ping: got %d %q", rec.Code, rec.Body.String())
	}
}

func init() {
	// Some CI environments pipe stderr into the test output; suppress
	// the chatty default pebble/embedded-db log by rerouting the root
	// log to os.Stdout via the package logger once at startup. We do
	// nothing here — tests should use t.Log for diagnostics — but
	// leaving this shim makes it trivial to tighten if a log stream
	// starts showing up in CI.
	_ = os.Stdout
}
