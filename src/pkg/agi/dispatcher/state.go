package dispatcher

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// State is the on-disk dispatcher state. It captures the last byte
// offset successfully posted to the AGI listener for a file source,
// and the last journal cursor for a journald source. The file is
// rewritten atomically (write-to-tmp + rename) so a crash during
// flush leaves either the previous or the new state, never a torn
// JSON document.
type State struct {
	// SourceID is a stable identifier for the (cluster,node,source)
	// tuple. It is sent on the wire as the ?source-id= query param
	// and used by the AGI listener to bind reconnects to the same
	// per-stream goroutine.
	SourceID string `json:"sourceID"`

	// FileOffset is the last byte offset in the source file that the
	// dispatcher has successfully posted (server-acked via 2xx).
	// Zero means "post from the current EOF" on first start unless
	// BackfillFromStart is true in the dispatcher Config.
	FileOffset int64 `json:"fileOffset,omitempty"`

	// FileInode is the inode the file had when FileOffset was
	// captured. If the inode changes on reopen we treat the file as
	// rotated and reset FileOffset to 0.
	FileInode uint64 `json:"fileInode,omitempty"`

	// JournalCursor is the last journald cursor successfully posted
	// (only used for journal sources).
	JournalCursor string `json:"journalCursor,omitempty"`

	// UpdatedAt is informational; it is the wall-clock of the last
	// successful flush.
	UpdatedAt time.Time `json:"updatedAt"`
}

// stateStore wraps a State with a file-backed flush. Concurrent
// callers may call Update repeatedly; Flush is idempotent and safe.
type stateStore struct {
	path string
	mu   sync.Mutex
	cur  State
	// dirty is true when an in-memory mutation hasn't yet been
	// flushed to disk.
	dirty bool
}

func newStateStore(path string, sourceID string) (*stateStore, error) {
	s := &stateStore{path: path}
	if path == "" {
		s.cur = State{SourceID: sourceID}
		return s, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir state dir: %w", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("read state: %w", err)
		}
		s.cur = State{SourceID: sourceID}
		return s, nil
	}
	if len(b) == 0 {
		s.cur = State{SourceID: sourceID}
		return s, nil
	}
	if err := json.Unmarshal(b, &s.cur); err != nil {
		// Corrupt state file: treat as empty so the dispatcher can
		// make progress; rotation/resume will simply restart from
		// the source's tail rather than backfill duplicates.
		s.cur = State{SourceID: sourceID}
		return s, nil
	}
	if s.cur.SourceID == "" {
		s.cur.SourceID = sourceID
	}
	return s, nil
}

func (s *stateStore) Snapshot() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur
}

func (s *stateStore) UpdateFile(offset int64, inode uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur.FileOffset = offset
	s.cur.FileInode = inode
	s.cur.UpdatedAt = time.Now().UTC()
	s.dirty = true
}

func (s *stateStore) UpdateJournal(cursor string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cur.JournalCursor = cursor
	s.cur.UpdatedAt = time.Now().UTC()
	s.dirty = true
}

// Flush writes the in-memory state to disk if it has changed since
// the previous successful flush. The write is atomic: a temp file is
// written and then renamed over the target. Returns nil if there is
// nothing to do.
func (s *stateStore) Flush() error {
	s.mu.Lock()
	if !s.dirty || s.path == "" {
		s.mu.Unlock()
		return nil
	}
	cur := s.cur
	s.dirty = false
	s.mu.Unlock()

	b, err := json.MarshalIndent(cur, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("write state tmp: %w", err)
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename state: %w", err)
	}
	return nil
}
