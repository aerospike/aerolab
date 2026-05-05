package livelisten

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type offsetStore struct {
	path   string
	mu     sync.Mutex
	data   map[string]int64
	dirty  bool
	stopCh chan struct{}
	done   chan struct{}
}

func newOffsetStore(path string) *offsetStore {
	s := &offsetStore{
		path:   path,
		data:   make(map[string]int64),
		stopCh: make(chan struct{}),
		done:   make(chan struct{}),
	}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &s.data)
		if s.data == nil {
			s.data = make(map[string]int64)
		}
	}
	go s.loopFlush()
	return s
}

func (s *offsetStore) loopFlush() {
	defer close(s.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			s.flush()
			return
		case <-ticker.C:
			s.flush()
		}
	}
}

func (s *offsetStore) Stop() {
	close(s.stopCh)
	<-s.done
}

func (s *offsetStore) flush() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.dirty {
		return
	}
	dir := filepath.Dir(s.path)
	_ = os.MkdirAll(dir, 0755)
	b, err := json.Marshal(s.data)
	if err != nil {
		return
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return
	}
	_ = os.Rename(tmp, s.path)
	s.dirty = false
}

func (s *offsetStore) get(id string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[id]
}

func (s *offsetStore) set(id string, off int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = off
	s.dirty = true
}
