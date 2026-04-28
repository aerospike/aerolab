package plugin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// TestStopCPUProfile_NoRunning asserts the no-op contract: calling
// StopCPUProfile on a plugin that never started a profile must not
// panic, must not touch the filesystem, and must report "" so the
// caller can distinguish "nothing flushed" from a real path.
func TestStopCPUProfile_NoRunning(t *testing.T) {
	p := newCPUProfileTestPlugin(t, "")
	if got := p.StopCPUProfile(); got != "" {
		t.Fatalf("StopCPUProfile with no profile running: got %q want \"\"", got)
	}
}

// TestStopCPUProfile_FlushesValidProfile starts a profile, runs a tiny
// CPU-bound workload to guarantee a non-empty sample buffer, then
// stops and verifies the on-disk file is a complete (gzipped) pprof.
// This is the regression test for the original "0-byte file" bug:
// before the StopCPUProfile-without-Close API existed there was no
// way to flush samples to disk short of process exit.
func TestStopCPUProfile_FlushesValidProfile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cpu.pprof")

	p := newCPUProfileTestPlugin(t, target)

	if err := p.StartCPUProfile(); err != nil {
		t.Fatalf("StartCPUProfile: %s", err)
	}
	burnCPUForProfile()

	flushed := p.StopCPUProfile()
	if flushed != target {
		t.Fatalf("StopCPUProfile path: got %q want %q", flushed, target)
	}
	assertNonEmptyPprof(t, flushed)

	// Idempotency: a second call after stop must be a no-op.
	if got := p.StopCPUProfile(); got != "" {
		t.Fatalf("second StopCPUProfile: got %q want \"\"", got)
	}
}

// TestRotateCPUProfile_NoConfig asserts the disabled-config contract:
// rotation must be a silent no-op when CPUProfilingOutputFile is
// empty (e.g. operator did not pass --plugin-cpu-profiling). A
// SIGUSR1 in this state should never error.
func TestRotateCPUProfile_NoConfig(t *testing.T) {
	p := newCPUProfileTestPlugin(t, "")
	rotated, err := p.RotateCPUProfile()
	if err != nil {
		t.Fatalf("RotateCPUProfile with empty config: %s", err)
	}
	if rotated != "" {
		t.Fatalf("RotateCPUProfile with empty config: got %q want \"\"", rotated)
	}
}

// TestRotateCPUProfile_BeforeStart asserts that the very first SIGUSR1
// (received before the deferred coordinator goroutine had a chance to
// StartCPUProfile) just starts a fresh profile and reports "" for
// "nothing rotated". This is the documented behaviour for the
// "operator hits the rotate button at t=0" edge case.
func TestRotateCPUProfile_BeforeStart(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cpu.pprof")

	p := newCPUProfileTestPlugin(t, target)

	rotated, err := p.RotateCPUProfile()
	if err != nil {
		t.Fatalf("RotateCPUProfile: %s", err)
	}
	if rotated != "" {
		t.Fatalf("first rotate: got %q want \"\" (nothing to flush)", rotated)
	}
	if !p.pprofRunning {
		t.Fatalf("first rotate: expected pprofRunning=true after restart")
	}

	// The configured path now points at the live (in-memory)
	// profile and must exist on disk even though it's 0 bytes.
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("configured path missing after first rotate: %s", err)
	}

	// Drain the live profile so the test doesn't leak the
	// process-global pprof state into a sibling test.
	if got := p.StopCPUProfile(); got != target {
		t.Fatalf("cleanup StopCPUProfile: got %q want %q", got, target)
	}
}

// TestRotateCPUProfile_StopRotateStart is the headline regression
// test: it exercises the full SIGUSR1 contract end to end.
//
//  1. Start a profile, do CPU-bound work.
//  2. Rotate. Assert: returned path is timestamp-suffixed, contains
//     a valid pprof, and is NOT the configured path; the configured
//     path now hosts the new (live, 0-byte) profile.
//  3. Do more CPU-bound work, rotate again. Assert: a second
//     timestamp-suffixed file exists with valid pprof data, distinct
//     from the first rotation. Two consecutive rotates must produce
//     two independent on-disk profiles.
func TestRotateCPUProfile_StopRotateStart(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cpu.pprof")

	p := newCPUProfileTestPlugin(t, target)

	if err := p.StartCPUProfile(); err != nil {
		t.Fatalf("StartCPUProfile: %s", err)
	}
	burnCPUForProfile()

	rotated1, err := p.RotateCPUProfile()
	if err != nil {
		t.Fatalf("first RotateCPUProfile: %s", err)
	}
	if rotated1 == "" || rotated1 == target {
		t.Fatalf("first rotated path: got %q (target=%q)", rotated1, target)
	}
	if !strings.HasPrefix(rotated1, target+".") {
		t.Fatalf("rotated path is not a timestamp-suffixed sibling: got %q want prefix %q.", rotated1, target)
	}
	assertNonEmptyPprof(t, rotated1)
	if !p.pprofRunning {
		t.Fatalf("after rotate: expected pprofRunning=true")
	}

	// Live (post-rotate) profile sits at the configured path.
	if _, err := os.Stat(target); err != nil {
		t.Fatalf("configured path missing after rotate: %s", err)
	}

	burnCPUForProfile()

	rotated2, err := p.RotateCPUProfile()
	if err != nil {
		t.Fatalf("second RotateCPUProfile: %s", err)
	}
	if rotated2 == "" || rotated2 == rotated1 {
		t.Fatalf("second rotated path: got %q (first=%q)", rotated2, rotated1)
	}
	assertNonEmptyPprof(t, rotated2)

	// Cleanup: drain the now-running third profile.
	if got := p.StopCPUProfile(); got != target {
		t.Fatalf("cleanup StopCPUProfile: got %q want %q", got, target)
	}
}

// TestRotateCPUProfile_ConcurrentSafe fires N rotates concurrently
// and asserts the mutex really serialises them. Without
// stopCPUProfileLocked + the held mutex around stop+rename, two
// rotates racing each other could either: (a) call pprof.StopCPUProfile
// against a profile the other one already stopped, (b) os.Rename the
// same just-flushed file twice (the second loses), or (c) leave
// pprofRunning desynced from the runtime profiler. Pass criteria:
// every successful rotate returns either "" (came in before the
// first start completed) or a unique non-empty path that exists on
// disk and parses as a non-empty pprof.
func TestRotateCPUProfile_ConcurrentSafe(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "cpu.pprof")

	p := newCPUProfileTestPlugin(t, target)

	if err := p.StartCPUProfile(); err != nil {
		t.Fatalf("StartCPUProfile: %s", err)
	}
	burnCPUForProfile()

	const N = 8
	type result struct {
		path string
		err  error
	}
	results := make(chan result, N)
	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			path, err := p.RotateCPUProfile()
			results <- result{path: path, err: err}
		}()
	}
	wg.Wait()
	close(results)

	seen := map[string]bool{}
	for r := range results {
		if r.err != nil {
			t.Fatalf("concurrent rotate returned error: %s", r.err)
		}
		if r.path == "" {
			continue // first-rotate-before-start; documented
		}
		if seen[r.path] {
			t.Fatalf("duplicate rotated path: %q", r.path)
		}
		seen[r.path] = true
		assertNonEmptyPprof(t, r.path)
	}
	// At least one rotate must have produced a real file; otherwise
	// the test didn't exercise the happy path at all.
	if len(seen) == 0 {
		t.Fatalf("no rotated profiles produced; concurrency test did not exercise happy path")
	}

	// Cleanup the live profile.
	if got := p.StopCPUProfile(); got != target {
		t.Fatalf("cleanup StopCPUProfile: got %q want %q", got, target)
	}
}

// TestRotateCPUProfile_RestartFailureWraps asserts the contract for
// the "no prior profile + StartCPUProfile fails" branch. Pointing the
// configured path at a non-existent parent directory makes the
// os.Create inside StartCPUProfile fail with ENOENT; rotate must
// surface that as a wrapped error and report rotated="" (nothing was
// flushed to rename in the first place).
func TestRotateCPUProfile_RestartFailureWraps(t *testing.T) {
	target := filepath.Join(t.TempDir(), "does", "not", "exist", "cpu.pprof")

	p := newCPUProfileTestPlugin(t, target)

	rotated, err := p.RotateCPUProfile()
	if err == nil {
		t.Fatalf("RotateCPUProfile against missing parent dir: expected error, got nil (rotated=%q)", rotated)
	}
	if rotated != "" {
		t.Fatalf("rotated path with no prior profile: got %q want \"\"", rotated)
	}
	if !strings.Contains(err.Error(), "rotate cpu profile: restart") {
		t.Fatalf("expected wrapped restart error, got: %s", err)
	}
}

// newCPUProfileTestPlugin returns a minimal Plugin with the supplied
// CPUProfilingOutputFile path (use "" to disable). It deliberately
// avoids Init/InitWithDB because those wire up the db, the cache
// refresher and the HTTP server — none of which the pprof tests
// need. The tradeoff: callers MUST NOT call p.Close() on the
// returned plugin (Close drains p.bg and p.handlers, which is fine,
// but p.done is never closed by Close while the test is running and
// re-entry is harmless — still, keep tests simple by skipping Close
// and stopping any live profile explicitly).
func newCPUProfileTestPlugin(t *testing.T, cpuProfilePath string) *Plugin {
	t.Helper()
	cfg := &Config{}
	cfg.CPUProfilingOutputFile = cpuProfilePath
	p := &Plugin{
		config: cfg,
		done:   make(chan struct{}),
	}
	t.Cleanup(func() {
		// Belt-and-braces: if a test forgot to drain the live
		// profile, do it here so the next test gets a clean
		// process-global runtime/pprof state.
		_ = p.StopCPUProfile()
	})
	return p
}

// burnCPUForProfile spins on a tight loop long enough for at least
// one runtime/pprof CPU sample tick (default 100Hz, so ~10ms is
// adequate; we burn for 50ms to be safe on slow CI). Without any
// samples the resulting profile would be a valid-but-minimal pprof
// — we want assertNonEmptyPprof to see actual function-level data.
func burnCPUForProfile() {
	deadline := time.Now().Add(50 * time.Millisecond)
	x := 1
	for time.Now().Before(deadline) {
		// Tight integer math; keep the compiler from
		// optimising the loop away by writing to x.
		x = (x*1664525 + 1013904223) & 0x7fffffff
	}
	// Reference x to guarantee the loop has observable effect.
	_ = x
}

// assertNonEmptyPprof verifies path exists, is non-zero in size, and
// begins with the gzip magic number 0x1f 0x8b. runtime/pprof writes
// CPU profiles as gzipped protobufs; the magic check is enough to
// distinguish a "the writer was flushed" file from the original
// 0-byte bug without pulling in a full pprof parser dependency.
func assertNonEmptyPprof(t *testing.T, path string) {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat %q: %s", path, err)
	}
	if st.Size() == 0 {
		t.Fatalf("pprof file %q is 0 bytes — StopCPUProfile did not flush", path)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %q: %s", path, err)
	}
	defer f.Close()
	hdr := make([]byte, 2)
	if _, err := f.Read(hdr); err != nil && !errors.Is(err, os.ErrClosed) {
		t.Fatalf("read header of %q: %s", path, err)
	}
	if hdr[0] != 0x1f || hdr[1] != 0x8b {
		t.Fatalf("pprof file %q is not gzipped (header %x %x); not a valid runtime/pprof CPU profile", path, hdr[0], hdr[1])
	}
}
