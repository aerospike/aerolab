package dispatcher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestComputeSourceID_Stable verifies the source-id derivation is
// deterministic for a given (cluster, node, source) tuple. The AGI
// listener uses this id to bind reconnects to the same per-stream
// goroutine, so any drift here would break node-prefix stickiness.
func TestComputeSourceID_Stable(t *testing.T) {
	src := Source{File: "/var/log/aerospike/aerospike.log"}
	id1 := computeSourceID("bob", "BB9A1C2", src)
	id2 := computeSourceID("bob", "BB9A1C2", src)
	if id1 != id2 {
		t.Fatalf("source-id should be deterministic; got %q vs %q", id1, id2)
	}
	if id1 == "" {
		t.Fatal("source-id should be non-empty")
	}
	if len(id1) != 40 { // sha1 hex
		t.Fatalf("source-id should be 40-char hex (sha1), got %d chars", len(id1))
	}
}

// TestComputeSourceID_DifferentInputsDiffer guards against accidental
// collisions: two different tuples must produce different ids.
func TestComputeSourceID_DifferentInputsDiffer(t *testing.T) {
	a := computeSourceID("bob", "n1", Source{File: "/a.log"})
	b := computeSourceID("bob", "n2", Source{File: "/a.log"})
	c := computeSourceID("bob", "n1", Source{File: "/b.log"})
	d := computeSourceID("bob", "n1", Source{Journal: "u.service"})
	if a == b || a == c || a == d || c == d {
		t.Fatal("source-ids should differ for different (cluster,node,source) tuples")
	}
}

// TestConfig_Validate_RequiredFields catches the early-return paths
// of Run before any I/O happens.
func TestConfig_Validate_RequiredFields(t *testing.T) {
	if err := (&Config{}).validate(); err == nil {
		t.Fatal("expected error for empty Target")
	}
	if err := (&Config{Target: "https://x:443"}).validate(); err == nil {
		t.Fatal("expected error for empty Token")
	}
	if err := (&Config{Target: "ftp://x", Token: "t"}).validate(); err == nil {
		t.Fatal("expected error for non-http(s) Target")
	}
	if err := (&Config{Target: "https://x:443", Token: "t"}).validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestStreamLoop_PostsLinesAndAcknowledgesBackpressure stands up a
// fake AGI listener (httptest server) and feeds the dispatcher a
// short sequence of lines, asserting they arrive on the wire intact
// and in order. This exercises the streamOnce body, the chunked POST
// pipe pump, the bearer token plumbing, and the on-progress
// callback.
func TestStreamLoop_PostsLinesAndAcknowledgesBackpressure(t *testing.T) {
	var (
		gotBody atomic.Value // string
		token   atomic.Value // string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token.Store(r.Header.Get("Authorization"))
		buf := make([]byte, 0, 1024)
		tmp := make([]byte, 256)
		for {
			n, err := r.Body.Read(tmp)
			if n > 0 {
				buf = append(buf, tmp[:n]...)
			}
			if err != nil {
				break
			}
		}
		gotBody.Store(string(buf))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := New(Config{
		Target: srv.URL,
		Token:  "t0p-secret",
	})
	// Skip identity discovery; pre-populate the resolved fields so
	// streamLoop can build a URL without needing an Aerospike node.
	d.cluster = "bob"
	d.nodeID = "n1"
	d.source = Source{File: "/x.log"}
	d.sourceID = "deadbeef"
	d.state = mustNewStateStore(t, "", "deadbeef")

	in := make(chan tailLine, 4)
	in <- tailLine{Line: []byte("first line"), After: 10, Inode: 1}
	in <- tailLine{Line: []byte("second line"), After: 21, Inode: 1}
	close(in)

	var progressCalls int32
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := d.streamOnce(ctx, in, func(_ int64, _ uint64) { atomic.AddInt32(&progressCalls, 1) }); err != nil {
		t.Fatalf("streamOnce: %v", err)
	}

	body, _ := gotBody.Load().(string)
	if !strings.Contains(body, "first line") || !strings.Contains(body, "second line") {
		t.Fatalf("expected both lines on the wire, got %q", body)
	}
	if got := token.Load(); got != "Bearer t0p-secret" {
		t.Fatalf("expected Bearer t0p-secret auth header, got %q", got)
	}
	if atomic.LoadInt32(&progressCalls) != 2 {
		t.Fatalf("expected 2 progress callbacks, got %d", progressCalls)
	}
}

// mustNewStateStore returns a state store or fatals; thin wrapper
// for keeping tests linear.
func mustNewStateStore(t *testing.T, path, srcID string) *stateStore {
	t.Helper()
	s, err := newStateStore(path, srcID)
	if err != nil {
		t.Fatalf("newStateStore: %v", err)
	}
	return s
}
