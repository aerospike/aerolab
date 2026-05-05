package livelisten

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// offsetStore is the per-stream byte-offset checkpoint backing the
// dispatcher resume contract. Active streams update an in-memory
// map under a mutex; a 1Hz background flusher writes the JSON to
// OffsetsPath atomically (write to .tmp + rename).
//
// JSON format on disk:
//   { "<source-id>": <last-offset>, ... }
//
// The dispatcher reads this on connect via a separate
// /agi/ingest/offsets endpoint (TODO: not implemented yet — the
// dispatcher's own state file is the immediate source of truth;
// the AGI-side checkpoint is a backup so a fresh dispatcher
// install can pick up where the old one left off).
type offsetStore struct {
	path string

	mu        sync.Mutex
	data      map[string]int64
	dirty     bool
	startOnce sync.Once
}

func newOffsetStore(path string) *offsetStore {
	return &offsetStore{
		path: path,
		data: make(map[string]int64),
	}
}

func (o *offsetStore) start(ctx context.Context) {
	o.startOnce.Do(func() {
		o.load()
		go o.flushLoop(ctx)
	})
}

// load reads the checkpoint at o.path. Missing file is normal on
// first ever start; corrupted JSON is logged and ignored so a stale
// checkpoint can never block the listener.
func (o *offsetStore) load() {
	if o.path == "" {
		return
	}
	b, err := os.ReadFile(o.path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("WARN: livelisten offsets: read %s: %s", o.path, err)
		}
		return
	}
	m := make(map[string]int64)
	if err := json.Unmarshal(b, &m); err != nil {
		log.Printf("WARN: livelisten offsets: parse %s: %s; starting empty", o.path, err)
		return
	}
	o.mu.Lock()
	o.data = m
	o.mu.Unlock()
}

func (o *offsetStore) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = o.flushNow()
			return
		case <-ticker.C:
			_ = o.flushNow()
		}
	}
}

func (o *offsetStore) flushNow() error {
	if o.path == "" {
		return nil
	}
	o.mu.Lock()
	if !o.dirty {
		o.mu.Unlock()
		return nil
	}
	cp := make(map[string]int64, len(o.data))
	for k, v := range o.data {
		cp[k] = v
	}
	o.dirty = false
	o.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(o.path), 0755); err != nil {
		return err
	}
	tmp := o.path + ".tmp"
	b, err := json.Marshal(cp)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, o.path)
}

// set unconditionally updates the offset for a source-id.
func (o *offsetStore) set(sourceID string, off int64) {
	o.mu.Lock()
	o.data[sourceID] = off
	o.dirty = true
	o.mu.Unlock()
}

// setIfHigher only advances an offset; useful when the dispatcher
// announces its own resume offset on (re)connect and we want to
// keep the higher of the two.
func (o *offsetStore) setIfHigher(sourceID string, off int64) {
	o.mu.Lock()
	if cur := o.data[sourceID]; off > cur {
		o.data[sourceID] = off
		o.dirty = true
	}
	o.mu.Unlock()
}

// get returns the last-known offset for a source-id (0 when not
// present).
func (o *offsetStore) get(sourceID string) int64 {
	o.mu.Lock()
	v := o.data[sourceID]
	o.mu.Unlock()
	return v
}
