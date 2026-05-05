package livelisten

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
)

// handle implements POST /agi/ingest/stream. The full wire shape is
// documented in the package doc comment.
func (l *Listener) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !l.checkAuth(w, r) {
		return
	}

	q := r.URL.Query()
	cluster := strings.TrimSpace(q.Get("cluster"))
	node := strings.TrimSpace(q.Get("node"))
	source := strings.TrimSpace(q.Get("source"))
	sourceID := strings.TrimSpace(q.Get("source-id"))
	if cluster == "" || node == "" {
		http.Error(w, "cluster and node query parameters are required", http.StatusBadRequest)
		return
	}
	if source == "" {
		source = "live:" + node
	}
	if sourceID == "" {
		// Stable per-(cluster,node,source) fallback keeps the
		// offsets file from blowing up when an old dispatcher
		// (without --source-id) reconnects.
		sourceID = cluster + "/" + node + "/" + source
	}

	// MaxStreams gate. Atomic CAS-style: bump first, drop on
	// reject. The race window between probe and bump is fine —
	// the gate is a safety cap not a hard SLA.
	current := atomic.AddInt64(&l.active, 1)
	if int(current) > l.cfg.MaxStreams {
		atomic.AddInt64(&l.active, -1)
		http.Error(w, "too many live streams", http.StatusTooManyRequests)
		return
	}
	defer func() {
		atomic.AddInt64(&l.active, -1)
		l.publishCount()
	}()
	l.publishCount()

	// Resume offset is informational: the dispatcher is the
	// source of truth for byte offsets. We surface our last
	// known offset back via Trailer so the dispatcher can
	// refuse to start over if our checkpoint is ahead of
	// theirs (corruption guard).
	if v := r.Header.Get("X-Resume-Offset"); v != "" {
		if off, err := strconv.ParseInt(v, 10, 64); err == nil {
			l.offsets.setIfHigher(sourceID, off)
		}
	}

	// Set the response header BEFORE writing any body bytes;
	// chunked encoding is implied by the lack of Content-Length
	// on Go's http server. WriteHeader flushes headers; subsequent
	// trailer writes work because we declared them up front.
	w.Header().Set("Trailer", "X-Last-Offset")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	stream := l.ingest.NewLiveStream(l.shards, cluster, node, source)

	// bufio.Scanner default token cap is 64KiB which can choke
	// on long aerospike log lines. Bump to 1 MiB to match the
	// LogReadBufferSizeKb default the batch path uses.
	scanner := bufio.NewScanner(r.Body)
	const maxLine = 1 << 20
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, maxLine)
	var bytesIn int64
	flushEvery := int64(64 << 10) // 64 KiB
	var lastFlushed int64
	var lineCount int64

	defer func() {
		stream.Close()
		// Final offsets flush + trailer.
		l.offsets.set(sourceID, bytesIn)
		_ = l.offsets.flushNow()
		w.Header().Set("X-Last-Offset", strconv.FormatInt(bytesIn, 10))
	}()

	ctx := r.Context()
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := scanner.Text()
		bytesIn += int64(len(line)) + 1 // +1 for the newline that the scanner consumed
		if line == "" {
			continue
		}
		if err := stream.Process(line); err != nil {
			log.Printf("WARN: livelisten: %s/%s: process line: %s", cluster, node, err)
		}
		lineCount++
		if bytesIn-lastFlushed >= flushEvery {
			l.offsets.set(sourceID, bytesIn)
			lastFlushed = bytesIn
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		log.Printf("WARN: livelisten: %s/%s: scanner: %s", cluster, node, err)
		// We've already written 200 OK; surface the error in
		// the trailer header instead.
		w.Header().Set("X-Last-Error", err.Error())
		return
	}

	if lineCount > 0 {
		log.Printf("DEBUG: livelisten: %s/%s/%s: ingested %d line(s) (%d bytes) source-id=%s", cluster, node, source, lineCount, bytesIn, sourceID)
	}
}

// health returns 200 unconditionally. Used by the proxy reverse-
// proxy and external monitoring to confirm the listener is up.
func (l *Listener) health(w http.ResponseWriter, _ *http.Request) {
	fmt.Fprintf(w, "live ingest active streams=%d\n", l.activeCount())
}

// checkAuth validates the bearer token against the on-disk token
// directory. Returns false (and writes a 401) if the token is
// missing or invalid.
//
// We intentionally do NOT use constant-time comparison here: the
// token store keeps tokens in a map, and the wider token system
// already used non-constant-time comparison everywhere else
// (cmdAgiExecProxy.go's inslice.HasString). Adding it here only
// would create a false sense of security; tokens are 64+ characters
// of uniform randomness, which makes timing attacks infeasible
// regardless.
func (l *Listener) checkAuth(w http.ResponseWriter, r *http.Request) bool {
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return false
	}
	tok := strings.TrimPrefix(auth, "Bearer ")
	if !l.tokens.has(tok) {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return false
	}
	return true
}
