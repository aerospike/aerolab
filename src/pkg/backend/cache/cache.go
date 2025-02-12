package cache

import (
	"encoding/json"
	"errors"
	"os"
	"path"

	"github.com/aerospike/aerolab/pkg/file"
)

type Cache struct {
	Enabled bool   `yaml:"Enabled" json:"Enabled"`
	Dir     string `yaml:"Dir" json:"Dir"`
}

var ErrNoCacheFile = errors.New("cache file not found")

func (b *Cache) Delete() error {
	if b == nil || !b.Enabled || b.Dir == "" {
		return nil
	}
	return os.RemoveAll(b.Dir)
}

func (b *Cache) Get(name string, dataPointer interface{}) error {
	if b == nil || !b.Enabled || b.Dir == "" {
		return nil
	}
	fname := path.Join(b.Dir, name+".json")
	if _, err := os.Stat(fname); err != nil && os.IsNotExist(err) {
		return ErrNoCacheFile
	}
	f, err := os.Open(fname)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(dataPointer)
}

func (b *Cache) Store(name string, data interface{}) error {
	if b == nil || !b.Enabled || b.Dir == "" {
		return nil
	}
	os.MkdirAll(b.Dir, 0755)
	fname := path.Join(b.Dir, name+".json")
	return file.StoreJSON(fname, ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, data)
}
