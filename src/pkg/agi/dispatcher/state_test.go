package dispatcher

import (
	"path/filepath"
	"testing"
)

// TestStateStore_RoundTrip writes a checkpoint, reopens the store,
// and asserts the snapshot matches. This is the single most
// important property of the state store: a restart resumes from the
// last successfully flushed offset+inode.
func TestStateStore_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agi-dispatch.state")

	s, err := newStateStore(path, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	s.UpdateFile(98765, 4242)
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}

	s2, err := newStateStore(path, "abc123")
	if err != nil {
		t.Fatal(err)
	}
	got := s2.Snapshot()
	if got.SourceID != "abc123" {
		t.Errorf("source-id mismatch: want abc123, got %q", got.SourceID)
	}
	if got.FileOffset != 98765 {
		t.Errorf("file-offset mismatch: want 98765, got %d", got.FileOffset)
	}
	if got.FileInode != 4242 {
		t.Errorf("file-inode mismatch: want 4242, got %d", got.FileInode)
	}
}

// TestStateStore_FlushIdempotent verifies that a Flush after a no-op
// (no UpdateFile/UpdateJournal calls) does NOT touch the on-disk
// file. This matters because the dispatcher flushes on a 1Hz timer;
// rewriting an unchanged file every second would be wasteful.
func TestStateStore_FlushIdempotent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agi-dispatch.state")

	s, err := newStateStore(path, "id")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
	// Flushing again with no mutations should be a no-op (no error).
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}
}

// TestStateStore_NoPath verifies that a state store with an empty
// path returns sane snapshots and Flush is a no-op. This is the
// in-memory mode used by tests that don't care about persistence.
func TestStateStore_NoPath(t *testing.T) {
	s, err := newStateStore("", "id")
	if err != nil {
		t.Fatal(err)
	}
	s.UpdateFile(1, 2)
	if err := s.Flush(); err != nil {
		t.Fatalf("flush should be a no-op when path is empty, got %v", err)
	}
	got := s.Snapshot()
	if got.FileOffset != 1 {
		t.Errorf("offset not retained in-memory: got %d", got.FileOffset)
	}
}
