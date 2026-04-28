package plugin

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// debugServer wires a fresh test plugin to a real ServeMux + Server so
// the path-based dispatch (/debug/db/sets/{name}) actually exercises
// the registered routes. Direct httptest.NewRequest() against an
// individual handler bypasses the mux and would mask routing bugs.
func debugServer(t *testing.T) (*httptest.Server, *Plugin) {
	t.Helper()
	p := newTestPlugin(t)
	p.mux = http.NewServeMux()
	p.registerDebugHandlers()
	srv := httptest.NewServer(p.mux)
	t.Cleanup(srv.Close)
	return srv, p
}

func TestDebugInfo(t *testing.T) {
	srv, p := debugServer(t)
	resp, err := http.Get(srv.URL + "/debug/db/info")
	if err != nil {
		t.Fatalf("get: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var got struct {
		Path           string   `json:"path"`
		StorageVersion uint32   `json:"storageVersion"`
		SetCount       int      `json:"setCount"`
		Sets           []string `json:"sets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if got.StorageVersion != db.CurrentStorageVersion() {
		t.Fatalf("storageVersion: got %d want %d", got.StorageVersion, db.CurrentStorageVersion())
	}
	if got.Path != p.db.Path() {
		t.Fatalf("path: got %q want %q", got.Path, p.db.Path())
	}
	wantSets := map[string]bool{"labels": true, "metrics": true}
	if got.SetCount < 2 {
		t.Fatalf("setCount: got %d want >=2", got.SetCount)
	}
	for _, s := range got.Sets {
		delete(wantSets, s)
	}
	if len(wantSets) != 0 {
		t.Fatalf("missing sets: %v (got %v)", wantSets, got.Sets)
	}
}

func TestDebugListSets(t *testing.T) {
	srv, _ := debugServer(t)
	resp, err := http.Get(srv.URL + "/debug/db/sets")
	if err != nil {
		t.Fatalf("get: %s", err)
	}
	defer resp.Body.Close()
	var got []struct {
		Name       string `json:"name"`
		Columns    int    `json:"columns"`
		IndexedCol string `json:"indexedCol,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %s", err)
	}
	bySet := map[string]int{}
	idx := map[string]string{}
	for _, s := range got {
		bySet[s.Name] = s.Columns
		idx[s.Name] = s.IndexedCol
	}
	if bySet["metrics"] == 0 {
		t.Fatalf("expected metrics set, got %+v", got)
	}
	if idx["metrics"] != "timestamp" {
		t.Fatalf("metrics indexedCol: got %q want %q", idx["metrics"], "timestamp")
	}
}

func TestDebugSetSchema(t *testing.T) {
	srv, _ := debugServer(t)
	resp, err := http.Get(srv.URL + "/debug/db/sets/metrics")
	if err != nil {
		t.Fatalf("get: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var got struct {
		Name       string `json:"name"`
		IndexedCol string `json:"indexedCol"`
		Columns    []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			Indexed bool   `json:"indexed"`
		} `json:"columns"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if got.Name != "metrics" {
		t.Fatalf("name: %q", got.Name)
	}
	if got.IndexedCol != "timestamp" {
		t.Fatalf("indexedCol: %q", got.IndexedCol)
	}
	wantCols := map[string]string{
		"timestamp":   "int64",
		"ClusterName": "int64",
		"NodeIdent":   "int64",
		"latency_p99": "int64",
		"op_count":    "int64",
	}
	gotTypes := map[string]string{}
	for _, c := range got.Columns {
		gotTypes[c.Name] = c.Type
	}
	for n, wt := range wantCols {
		if gt := gotTypes[n]; gt != wt {
			t.Errorf("column %q: got type %q want %q", n, gt, wt)
		}
	}
}

func TestDebugSetSchemaNotFound(t *testing.T) {
	srv, _ := debugServer(t)
	resp, err := http.Get(srv.URL + "/debug/db/sets/does-not-exist")
	if err != nil {
		t.Fatalf("get: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d want 404", resp.StatusCode)
	}
}

func TestDebugSampleStreamsNDJSON(t *testing.T) {
	srv, _ := debugServer(t)
	resp, err := http.Get(srv.URL + "/debug/db/sample?set=metrics&limit=3")
	if err != nil {
		t.Fatalf("get: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	rows, meta := decodeNDJSON(t, resp.Body)
	if len(rows) != 3 {
		t.Fatalf("rows: got %d want 3", len(rows))
	}
	if !meta.Truncated {
		t.Fatalf("expected truncated=true (5 seeded rows, limit=3)")
	}
	if meta.Rows != 3 {
		t.Fatalf("meta.rowsReturned=%d want 3", meta.Rows)
	}
	if meta.Error != "" {
		t.Fatalf("unexpected error: %s", meta.Error)
	}
}

func TestDebugGetFound(t *testing.T) {
	srv, p := debugServer(t)
	// Find a real key by scanning.
	it := p.db.Scan("labels")
	defer it.Close()
	if !it.Next() {
		t.Fatal("expected at least one row in labels set")
	}
	key, _ := it.Record()

	resp, err := http.Get(srv.URL + "/debug/db/get?set=labels&key=" + key)
	if err != nil {
		t.Fatalf("get: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var got struct {
		Set   string     `json:"set"`
		Key   string     `json:"key"`
		Found bool       `json:"found"`
		Row   db.WireRow `json:"row"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if !got.Found {
		t.Fatalf("expected found=true for known key %q", key)
	}
	if _, ok := got.Row["json"]; !ok {
		t.Fatalf("expected 'json' column in labels row, got %+v", got.Row)
	}
}

func TestDebugGetMissing(t *testing.T) {
	srv, _ := debugServer(t)
	resp, err := http.Get(srv.URL + "/debug/db/get?set=metrics&key=does-not-exist")
	if err != nil {
		t.Fatalf("get: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d", resp.StatusCode)
	}
	var got struct {
		Found bool `json:"found"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %s", err)
	}
	if got.Found {
		t.Fatal("expected found=false")
	}
}

func TestDebugQueryHotPath(t *testing.T) {
	srv, _ := debugServer(t)
	// Query the metrics set with the same shape as the plugin's
	// timeseries hot path: indexed Between + Eq filter + projection.
	plan := `{
	  "set": "metrics",
	  "between": {"col":"timestamp","lo":{"int":0},"hi":{"int":99999999999999}},
	  "where":   {"and":[{"eq":{"col":"ClusterName","value":{"int":0}}},{"exists":{"col":"latency_p99"}}]},
	  "project": ["timestamp","latency_p99"],
	  "limit":   100
	}`
	resp, err := http.Post(srv.URL+"/debug/db/query", "application/json", strings.NewReader(plan))
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status: %d body=%s", resp.StatusCode, readAll(t, resp.Body))
	}
	rows, meta := decodeNDJSON(t, resp.Body)
	if meta.Error != "" {
		t.Fatalf("query error: %s", meta.Error)
	}
	if len(rows) == 0 {
		t.Fatalf("expected rows, got 0")
	}
	for _, r := range rows {
		if _, ok := r.Row["timestamp"]; !ok {
			t.Errorf("missing timestamp in projected row: %+v", r.Row)
		}
		if _, ok := r.Row["ClusterName"]; ok {
			t.Errorf("ClusterName should not be projected: %+v", r.Row)
		}
	}
}

func TestDebugQueryRejectsUnknownFields(t *testing.T) {
	srv, _ := debugServer(t)
	plan := `{"set":"metrics","wat":1}`
	resp, err := http.Post(srv.URL+"/debug/db/query", "application/json", strings.NewReader(plan))
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
}

func TestDebugQueryRejectsMissingSet(t *testing.T) {
	srv, _ := debugServer(t)
	resp, err := http.Post(srv.URL+"/debug/db/query", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("post: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status: got %d want 400", resp.StatusCode)
	}
}

func TestDebugQueryRejectsGet(t *testing.T) {
	srv, _ := debugServer(t)
	resp, err := http.Get(srv.URL + "/debug/db/query")
	if err != nil {
		t.Fatalf("get: %s", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("status: got %d want 405", resp.StatusCode)
	}
	if got := resp.Header.Get("Allow"); got != http.MethodPost {
		t.Fatalf("Allow header: got %q want %q", got, http.MethodPost)
	}
}

// --- helpers ---

type ndjsonRow struct {
	Key string     `json:"key"`
	Row db.WireRow `json:"row"`
}

type ndjsonMeta struct {
	Rows      int    `json:"rowsReturned"`
	Truncated bool   `json:"truncated"`
	Duration  string `json:"durationMs"`
	Error     string `json:"error,omitempty"`
}

// decodeNDJSON splits the NDJSON stream into row records and the
// trailing _meta record. The contract is "every line is a row except
// the final line, which carries _meta" — see streamRows in
// frontend_debug.go.
func decodeNDJSON(t *testing.T, r interface {
	Read(p []byte) (n int, err error)
}) ([]ndjsonRow, ndjsonMeta) {
	t.Helper()
	body := readAll(t, r)
	var rows []ndjsonRow
	var meta ndjsonMeta
	for _, line := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		if line == "" {
			continue
		}
		// Try meta first; it always carries a _meta key.
		var probe map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &probe); err != nil {
			t.Fatalf("ndjson: bad line %q: %s", line, err)
		}
		if raw, ok := probe["_meta"]; ok {
			if err := json.Unmarshal(raw, &meta); err != nil {
				t.Fatalf("ndjson: meta decode: %s", err)
			}
			continue
		}
		var row ndjsonRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatalf("ndjson: row decode %q: %s", line, err)
		}
		rows = append(rows, row)
	}
	return rows, meta
}

func readAll(t *testing.T, r interface {
	Read(p []byte) (n int, err error)
}) string {
	t.Helper()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			break
		}
	}
	return string(buf)
}

// Sanity: ensure the registered routes survive a fresh mux build (catches
// any future refactor that moves the registration off the per-plugin mux).
func TestDebugRoutesRegistered(t *testing.T) {
	srv, _ := debugServer(t)
	for _, path := range []string{"/debug/db/info", "/debug/db/sets"} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatalf("get %s: %s", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Errorf("%s: status %d", path, resp.StatusCode)
		}
	}
}

// Compile-time assertion: keep newTestPlugin's signature stable for
// any future addition; if it ever returns more than *Plugin, this
// file's debugServer will fail to compile and force an explicit
// update.
var _ = func(t *testing.T) *Plugin { return newTestPlugin(t) }

// Suppress the unused fmt import on the noagi build — still wire it
// here in case a future expansion of these tests needs templated
// error messages.
var _ = fmt.Sprintf
