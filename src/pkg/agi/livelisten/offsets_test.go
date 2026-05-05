package livelisten

import (
	"path/filepath"
	"testing"
)

// TestOffsets_RoundTrip writes a known offset, reopens the store
// and asserts the value persists. This is the core dispatcher resume
// contract: offsets observed during a run survive the restart.
func TestOffsets_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "offsets.json")

	o := newOffsetStore(path)
	o.set("src-A", 12345)
	o.set("src-B", 67890)
	if err := o.flushNow(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	o2 := newOffsetStore(path)
	o2.load()
	if got := o2.get("src-A"); got != 12345 {
		t.Errorf("src-A: want 12345, got %d", got)
	}
	if got := o2.get("src-B"); got != 67890 {
		t.Errorf("src-B: want 67890, got %d", got)
	}
}

// TestOffsets_SetIfHigher only advances; never goes backwards. This
// matters because the dispatcher and the AGI both track offsets
// independently and we want the higher value to win when they
// disagree.
func TestOffsets_SetIfHigher(t *testing.T) {
	o := newOffsetStore("")
	o.setIfHigher("k", 100)
	if got := o.get("k"); got != 100 {
		t.Errorf("after first setIfHigher: want 100, got %d", got)
	}
	o.setIfHigher("k", 50)
	if got := o.get("k"); got != 100 {
		t.Errorf("setIfHigher should not regress: want 100, got %d", got)
	}
	o.setIfHigher("k", 200)
	if got := o.get("k"); got != 200 {
		t.Errorf("setIfHigher should advance: want 200, got %d", got)
	}
}

// TestOffsets_FlushNoOpWhenClean confirms flushNow is a no-op when
// there is nothing to write — important since we flush every second
// even on idle streams.
func TestOffsets_FlushNoOpWhenClean(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "offsets.json")
	o := newOffsetStore(path)
	if err := o.flushNow(); err != nil {
		t.Fatalf("first flush should be a no-op when clean: %v", err)
	}
}
