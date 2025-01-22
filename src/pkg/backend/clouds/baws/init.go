package baws

import (
	"encoding/json"
	"os"
	"path"
	"slices"

	"github.com/aerospike/aerolab/pkg/backend"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/aerospike/aerolab/pkg/file"
)

type b struct {
	configDir   string
	credentials *clouds.AWS
	regions     []string
}

func init() {
	backend.RegisterBackend(backend.BackendTypeAWS, &b{})
}

func (s *b) SetConfig(dir string, credentials *clouds.Credentials) error {
	s.configDir = dir
	s.credentials = &credentials.AWS
	// read regions
	err := s.setConfigRegions()
	if err != nil {
		return err
	}
	return nil
}

func (s *b) setConfigRegions() error {
	regionsFile := path.Join(s.configDir, "regions.json")
	_, err := os.Stat(regionsFile)
	if err != nil && !os.IsNotExist(err) {
		// error reading
		return err
	}
	if err != nil {
		// file does not exist
		return nil
	}
	// read
	f, err := os.Open(regionsFile)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewDecoder(f).Decode(&s.regions)
}

func (s *b) ListEnabledZones() ([]string, error) {
	return s.regions, nil
}

func (s *b) EnableZones(names ...string) error {
	regions, err := s.ListEnabledZones()
	if err != nil {
		return err
	}
	regions = append(regions, names...)
	return file.StoreJSON(path.Join(s.configDir, "regions.json"), ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, regions)
}

func (s *b) DisableZones(names ...string) error {
	currentRegions, err := s.ListEnabledZones()
	if err != nil {
		return err
	}
	regions := []string{}
	for _, r := range currentRegions {
		if slices.Contains(names, r) {
			continue
		}
		regions = append(regions, r)
	}
	return file.StoreJSON(path.Join(s.configDir, "regions.json"), ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, regions)
}
