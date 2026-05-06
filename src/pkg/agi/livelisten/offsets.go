package livelisten

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

type offsetStore struct {
	path string
	mu   sync.Mutex
	data map[string]int64
}

func newOffsetStore(path string) *offsetStore {
	return &offsetStore{path: path, data: make(map[string]int64)}
}

func (s *offsetStore) Load() error {
	if s.path == "" {
		return nil
	}
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Unmarshal(b, &s.data)
}

func (s *offsetStore) Save() error {
	if s.path == "" {
		return nil
	}
	s.mu.Lock()
	copyData := make(map[string]int64, len(s.data))
	for k, v := range s.data {
		copyData[k] = v
	}
	s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(copyData, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *offsetStore) Set(sourceID string, offset int64) {
	if s == nil || sourceID == "" {
		return
	}
	s.mu.Lock()
	s.data[sourceID] = offset
	s.mu.Unlock()
}

func (s *offsetStore) Get(sourceID string) int64 {
	if s == nil || sourceID == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data[sourceID]
}
