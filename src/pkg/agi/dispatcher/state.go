package dispatcher

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type State struct {
	Offset int64 `json:"offset"`
}

func loadState(path string) (*State, error) {
	if path == "" {
		return &State{}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &State{}, nil
		}
		return nil, err
	}
	state := new(State)
	if err := json.Unmarshal(b, state); err != nil {
		return nil, err
	}
	return state, nil
}

func saveState(path string, state *State) error {
	if path == "" || state == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
