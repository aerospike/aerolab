package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// TestSharedDBIngestPluginIntegration exercises the X4 scenario: a
// single *db.DB handle shared by the plugin (via InitWithDB) and a
// second writer in the same process that simulates the ingest
// pipeline's writes. This is the contract the merged
// cmdAgiExecService depends on — Pebble's exclusive-lock invariant
// means two independent processes can't both open the same directory,
// so ingest+plugin run as one process and share a handle.
//
// The test verifies that:
//   - plugin.InitWithDB accepts an externally-opened handle.
//   - Writes made on the shared handle (mimicking what the ingest
//     package does via its own InitWithDB path) become visible to the
//     plugin's cache refresh and HTTP handlers without any cross-
//     process coordination.
//   - Plugin.Close does NOT close the shared DB (p.ownsDB==false);
//     the caller can keep using the handle afterwards.
func TestSharedDBIngestPluginIntegration(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agi-db")
	opts := db.DefaultOptions()
	opts.Path = dir

	d, err := db.Open(opts)
	if err != nil {
		t.Fatalf("db.Open: %s", err)
	}
	// Explicitly close at end of test so we can assert plugin.Close
	// did NOT close it (i.e. the next Put after p.Close still
	// succeeds). The order matters: plugin.Close → direct Put → d.Close.
	closed := false
	defer func() {
		if !closed {
			_ = d.Close()
		}
	}()

	yamlConfig := fmt.Sprintf(
		"db:\n  path: %q\n  enableWAL: false\nlabelsSetName: labels\ntimestampBinName: timestamp\n",
		dir,
	)
	cfg, err := MakeConfigReader(true, strings.NewReader(yamlConfig), false)
	if err != nil {
		t.Fatalf("MakeConfigReader: %s", err)
	}
	cfg.LogLevel = 4
	cfg.CacheRefreshInterval = time.Hour

	p, err := InitWithDB(cfg, d)
	if err != nil {
		t.Fatalf("InitWithDB: %s", err)
	}
	if p.ownsDB {
		t.Fatal("ownsDB must be false after InitWithDB")
	}
	if p.db != d {
		t.Fatal("plugin did not adopt the injected db handle")
	}

	// Write metadata + metric rows directly on the shared handle.
	// This stands in for ingest's putData / Put calls; the critical
	// behavior under test is cross-component visibility on a single
	// handle, not the ingest pipeline's orchestration.
	seedTestData(t, d, cfg.LabelsSetName)

	// Synchronously drive one cache refresh cycle so the HTTP
	// handlers observe the seeded state. The background refresher
	// would do this too but it's gated on CacheRefreshInterval
	// (1h above) — we don't want to wait.
	if err := p.cacheSetList(); err != nil {
		t.Fatalf("cacheSetList: %s", err)
	}
	if err := p.cacheBinList(); err != nil {
		t.Fatalf("cacheBinList: %s", err)
	}
	if err := p.cacheMetadataList(); err != nil {
		t.Fatalf("cacheMetadataList: %s", err)
	}

	// /metrics returns the list of sets the plugin knows about. Both
	// the labels set and the metrics set must be reachable via the
	// shared handle.
	req := httptest.NewRequest(http.MethodPost, "/metrics", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()
	p.handleMetrics(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/metrics status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var metricsResp []map[string]any
	if derr := json.NewDecoder(rec.Body).Decode(&metricsResp); derr != nil {
		t.Fatalf("decode /metrics: %s", derr)
	}
	seen := map[string]bool{}
	for _, r := range metricsResp {
		if v, ok := r["value"].(string); ok {
			seen[v] = true
		}
	}
	if !seen["labels"] || !seen["metrics"] {
		t.Fatalf("shared handle: plugin did not see seeded sets, got %+v", metricsResp)
	}

	// A real timeseries query against the shared handle must return
	// datapoints for the rows just written. This hits the primary
	// index (the metrics set was registered with timestamp Indexed)
	// so it also verifies the index was built against the shared
	// handle.
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
	req = httptest.NewRequest(http.MethodPost, "/query", strings.NewReader(body))
	rec = httptest.NewRecorder()
	p.handleQuery(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/query status: got %d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var qresp []map[string]any
	if derr := json.NewDecoder(rec.Body).Decode(&qresp); derr != nil {
		t.Fatalf("decode /query: %s", derr)
	}
	if len(qresp) == 0 {
		t.Fatalf("shared handle: expected at least one series, got %+v", qresp)
	}
	dps, _ := qresp[0]["datapoints"].([]any)
	if len(dps) == 0 {
		t.Fatalf("shared handle: expected datapoints in series, got %+v", qresp[0])
	}

	// Close the plugin. With ownsDB==false this must NOT close the
	// shared handle — a regression here would manifest as the Put
	// below failing with "db: closed".
	p.Close()

	if err := d.Put("metrics", "probe/after-close", db.Row{
		"timestamp":   db.Int(time.Now().UnixMilli()),
		"latency_p99": db.Int(999),
	}); err != nil {
		t.Fatalf("shared handle Put after plugin.Close failed — plugin.Close must not close the shared db: %s", err)
	}

	// Now the owner (the test) closes the handle.
	if err := d.Close(); err != nil {
		t.Fatalf("explicit db.Close: %s", err)
	}
	closed = true
}
