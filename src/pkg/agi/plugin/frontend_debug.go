package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/agi/db"
)

// registerDebugHandlers wires the read-only /debug/db/* endpoints onto
// the plugin's mux. These endpoints exist for operator debugging only:
// they share the plugin's listen address (default 127.0.0.1:8851) so
// they are localhost-only by default and never reach the public proxy.
//
// Endpoint family (all GET unless noted; all read-only):
//
//	GET  /debug/db/info             — version, path, stats
//	GET  /debug/db/sets             — list of set names (+ basic schema summary)
//	GET  /debug/db/sets/{name}      — full schema for one set
//	GET  /debug/db/sample?set=N     — first ?limit=N rows via Scan
//	GET  /debug/db/get?set=...&key=...
//	POST /debug/db/query            — JSON plan in body, NDJSON results out
//
// Writes (Put / Update / Delete / DropSet / DropColumn) are deliberately
// not exposed — debugging a running ingest pipeline by mutating its
// state is rarely what the operator wanted, and there is no rollback.
func (p *Plugin) registerDebugHandlers() {
	p.mux.HandleFunc("/debug/db/info", p.trackHandler(p.handleDebugInfo))
	p.mux.HandleFunc("/debug/db/sets", p.trackHandler(p.handleDebugSets))
	p.mux.HandleFunc("/debug/db/sets/", p.trackHandler(p.handleDebugSetSchema))
	p.mux.HandleFunc("/debug/db/sample", p.trackHandler(p.handleDebugSample))
	p.mux.HandleFunc("/debug/db/get", p.trackHandler(p.handleDebugGet))
	p.mux.HandleFunc("/debug/db/query", p.trackHandler(p.handleDebugQuery))
}

// debugMaxSampleLimit / debugMaxQueryLimit cap the worst case any
// single debug call can return. The query path defaults to 1000 when
// the caller forgets to set Limit; both are pure safety rails (a
// runaway curl loop should not OOM the plugin).
const (
	debugMaxSampleLimit  = 10_000
	debugMaxQueryLimit   = 100_000
	debugDefaultQueryLim = 1000
)

// debugInfoResponse is the JSON shape returned by /debug/db/info.
type debugInfoResponse struct {
	Path           string   `json:"path"`
	StorageVersion uint32   `json:"storageVersion"`
	SetCount       int      `json:"setCount"`
	Sets           []string `json:"sets"`
	Stats          db.Stats `json:"stats"`
}

func (p *Plugin) handleDebugInfo(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	sets := p.db.Sets()
	sort.Strings(sets)
	resp := debugInfoResponse{
		Path:           p.db.Path(),
		StorageVersion: db.CurrentStorageVersion(),
		SetCount:       len(sets),
		Sets:           sets,
		Stats:          p.db.Stats(),
	}
	writeJSON(w, http.StatusOK, resp)
}

// debugSetSummary is the per-set entry in /debug/db/sets.
type debugSetSummary struct {
	Name       string `json:"name"`
	Columns    int    `json:"columns"`
	IndexedCol string `json:"indexedCol,omitempty"`
}

func (p *Plugin) handleDebugSets(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	names := p.db.Sets()
	sort.Strings(names)
	out := make([]debugSetSummary, 0, len(names))
	for _, n := range names {
		cols, ok := p.db.SchemaOf(n)
		if !ok {
			continue
		}
		var idx string
		for _, c := range cols {
			if c.Indexed {
				idx = c.Name
				break
			}
		}
		out = append(out, debugSetSummary{
			Name:       n,
			Columns:    len(cols),
			IndexedCol: idx,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// debugColumnView is the JSON shape of a single column in a schema
// dump. It mirrors db.ColumnSpec but renders the type as a stable
// human-readable string ("int64", "float64", …) rather than the
// uint8 wire form, because the type tag is part of the operator
// contract — bumping db.ColumnType numbering shouldn't silently
// rewrite the debug output.
type debugColumnView struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Indexed bool   `json:"indexed,omitempty"`
}

type debugSetSchema struct {
	Name       string            `json:"name"`
	IndexedCol string            `json:"indexedCol,omitempty"`
	Columns    []debugColumnView `json:"columns"`
}

func (p *Plugin) handleDebugSetSchema(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/debug/db/sets/")
	if name == "" || strings.Contains(name, "/") {
		writeError(w, http.StatusBadRequest, "missing or invalid set name in path")
		return
	}
	cols, ok := p.db.SchemaOf(name)
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("set %q not found", name))
		return
	}
	out := debugSetSchema{Name: name, Columns: make([]debugColumnView, 0, len(cols))}
	for _, c := range cols {
		out.Columns = append(out.Columns, debugColumnView{
			Name:    c.Name,
			Type:    c.Type.String(),
			Indexed: c.Indexed,
		})
		if c.Indexed {
			out.IndexedCol = c.Name
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (p *Plugin) handleDebugSample(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	set := r.URL.Query().Get("set")
	if set == "" {
		writeError(w, http.StatusBadRequest, "missing required query parameter: set")
		return
	}
	limit := 100
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, "limit must be a non-negative integer")
			return
		}
		limit = n
	}
	if limit == 0 || limit > debugMaxSampleLimit {
		limit = debugMaxSampleLimit
	}
	if !p.db.SetExists(set) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("set %q not found", set))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	it := p.db.ScanContext(ctx, set)
	defer it.Close()

	streamRows(w, it, limit, "scan")
}

type debugGetResponse struct {
	Set   string      `json:"set"`
	Key   string      `json:"key"`
	Found bool        `json:"found"`
	Row   db.WireRow  `json:"row,omitempty"`
}

func (p *Plugin) handleDebugGet(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodGet) {
		return
	}
	q := r.URL.Query()
	set := q.Get("set")
	key := q.Get("key")
	if set == "" || key == "" {
		writeError(w, http.StatusBadRequest, "missing required query parameters: set and key")
		return
	}
	if !p.db.SetExists(set) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("set %q not found", set))
		return
	}
	row, err := p.db.Get(set, key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := debugGetResponse{Set: set, Key: key, Found: row != nil}
	if row != nil {
		resp.Row = db.FromRow(row)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (p *Plugin) handleDebugQuery(w http.ResponseWriter, r *http.Request) {
	if !methodAllowed(w, r, http.MethodPost) {
		return
	}
	if r.Body == nil {
		writeError(w, http.StatusBadRequest, "request body required (JSON query plan)")
		return
	}
	defer r.Body.Close()

	var q db.WireQuery
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&q); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("decode query: %s", err))
		return
	}
	if q.Set == "" {
		writeError(w, http.StatusBadRequest, "query must specify 'set'")
		return
	}
	if !p.db.SetExists(q.Set) {
		writeError(w, http.StatusNotFound, fmt.Sprintf("set %q not found", q.Set))
		return
	}

	limit := q.Limit
	if limit <= 0 {
		limit = debugDefaultQueryLim
	}
	if limit > debugMaxQueryLimit {
		limit = debugMaxQueryLimit
	}

	qb := p.db.Query(q.Set)
	if q.Between != nil {
		lo, err := q.Between.Lo.ToValue()
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("between.lo: %s", err))
			return
		}
		hi, err := q.Between.Hi.ToValue()
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("between.hi: %s", err))
			return
		}
		qb = qb.Between(q.Between.Col, lo, hi)
	}
	if q.Where != nil {
		expr, err := q.Where.ToExpr()
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("where: %s", err))
			return
		}
		if expr != nil {
			qb = qb.Where(expr)
		}
	}
	if len(q.Project) > 0 {
		qb = qb.Project(q.Project...)
	}

	// Cap the per-request runtime so a buggy debug call can't pin a
	// Pebble snapshot indefinitely. 60s mirrors plugin.yaml's
	// shutdownTimeout default — the slowest legitimate debug query
	// we expect to run.
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	it := qb.Run(ctx)
	defer it.Close()

	streamRows(w, it, limit, "query")
}

// --- helpers ---

// streamRows writes an Iter as a single newline-delimited JSON
// document family on w:
//
//	{"key":"...","row":{...}}\n
//	{"key":"...","row":{...}}\n
//	{"_meta":{"rowsReturned":N,"truncated":bool,"durationMs":...}}\n
//
// The trailing _meta record is always present so a client can tell
// "iterator drained cleanly" from "iterator hit the limit and there
// would have been more." On iterator error the meta record carries
// {"_meta":{"error":"..."}} and the HTTP status is still 200 because
// any rows already streamed are real — switching to a 5xx mid-stream
// would just confuse curl. The client must inspect the trailing
// _meta record for the authoritative outcome.
func streamRows(w http.ResponseWriter, it db.Iter, limit int, kind string) {
	w.Header().Set("Content-Type", "application/x-ndjson; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	flusher, _ := w.(http.Flusher)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	start := time.Now()
	rows := 0
	truncated := false
	for it.Next() {
		key, row := it.Record()
		rec := struct {
			Key string     `json:"key"`
			Row db.WireRow `json:"row"`
		}{Key: key, Row: db.FromRow(row)}
		if err := enc.Encode(rec); err != nil {
			// Client likely closed the connection mid-stream;
			// nothing to do but stop. Logging is at DEBUG so
			// this is not noisy in normal operation.
			log.Printf("DEBUG: debug %s stream encode: %s", kind, err)
			return
		}
		rows++
		if rows >= limit {
			truncated = it.Next() // peek to see if there were more
			break
		}
	}
	meta := struct {
		Rows      int    `json:"rowsReturned"`
		Truncated bool   `json:"truncated"`
		Duration  string `json:"durationMs"`
		Error     string `json:"error,omitempty"`
	}{
		Rows:      rows,
		Truncated: truncated,
		Duration:  fmt.Sprintf("%d", time.Since(start).Milliseconds()),
	}
	if err := it.Err(); err != nil {
		meta.Error = err.Error()
	}
	wrap := struct {
		Meta any `json:"_meta"`
	}{Meta: meta}
	//nolint:errcheck
	enc.Encode(wrap)
	if flusher != nil {
		flusher.Flush()
	}
}

// methodAllowed enforces the HTTP method for a debug route. It writes
// a 405 with an Allow header on mismatch and returns false. Returning
// the value rather than letting net/http's default handler respond
// keeps the error-shape consistent with writeError.
func methodAllowed(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeError(w, http.StatusMethodNotAllowed, fmt.Sprintf("method %s not allowed; want %s", r.Method, method))
	return false
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(body); err != nil {
		log.Printf("DEBUG: debug writeJSON encode: %s", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	type errBody struct {
		Error string `json:"error"`
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	//nolint:errcheck
	json.NewEncoder(w).Encode(errBody{Error: msg})
}
