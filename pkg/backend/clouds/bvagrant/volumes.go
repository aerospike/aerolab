package bvagrant

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/file"
)

// CreateVolumeParams is the vagrant-backend-specific payload expected in
// backends.CreateVolumeInput.BackendSpecificParams[backends.BackendTypeVagrant]. Vagrant
// volumes are just a host directory bind-mounted into instances (see parseSyncedFolders
// in vagrantfile.go), so there are no backend-specific knobs to carry.
type CreateVolumeParams struct{}

// volumeMeta is the on-disk metadata record for a single volume, stored at
// <configDir>/volumes/<name>/volume.json.
type volumeMeta struct {
	Name      string            `json:"name"`
	Owner     string            `json:"owner"`
	Tags      map[string]string `json:"tags"`
	Expires   time.Time         `json:"expires"`
	CreatedAt time.Time         `json:"createdAt"`
}

const volumeMetaFileName = "volume.json"

func (s *b) volumesDir() string {
	return filepath.Join(s.configDir, "volumes")
}

func (s *b) volumeDir(name string) string {
	return filepath.Join(s.volumesDir(), name)
}

func (s *b) volumeMetaFile(name string) string {
	return filepath.Join(s.volumeDir(name), volumeMetaFileName)
}

func loadVolumeMeta(path string) (*volumeMeta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := &volumeMeta{}
	if err := json.Unmarshal(raw, m); err != nil {
		return nil, fmt.Errorf("corrupt volume metadata %q: %w", path, err)
	}
	return m, nil
}

func (s *b) saveVolumeMeta(m *volumeMeta) error {
	dir := s.volumeDir(m.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	return file.StoreJSON(s.volumeMetaFile(m.Name), ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, m)
}

// listVolumeMetas scans volumesDir for every volume subdirectory containing a
// volume.json. A directory that can't be read/parsed is skipped with a log warning,
// matching listClusterMetas' best-effort scanning behavior.
func (s *b) listVolumeMetas() ([]*volumeMeta, error) {
	entries, err := os.ReadDir(s.volumesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var metas []*volumeMeta
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		m, err := loadVolumeMeta(filepath.Join(s.volumesDir(), e.Name(), volumeMetaFileName))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			if s.log != nil {
				s.log.Warn("skipping unreadable volume metadata for %q: %v", e.Name(), err)
			}
			continue
		}
		metas = append(metas, m)
	}
	return metas, nil
}

func volumeFromMeta(m *volumeMeta) *backends.Volume {
	tags := m.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	return &backends.Volume{
		BackendType:  backends.BackendTypeVagrant,
		VolumeType:   backends.VolumeTypeSharedDisk,
		Name:         m.Name,
		FileSystemId: m.Name,
		ZoneName:     "local",
		ZoneID:       "local",
		CreationTime: m.CreatedAt,
		Owner:        m.Owner,
		Tags:         tags,
		Expires:      m.Expires,
		State:        backends.VolumeStateAvailable,
	}
}

// CreateVolume creates a new shared-directory volume at <configDir>/volumes/<name>,
// writing its metadata to volume.json. A volume with the same name already existing is
// an error (vagrant volumes are just directories, so "already exists" is unambiguous).
func (s *b) CreateVolume(input *backends.CreateVolumeInput) (*backends.CreateVolumeOutput, error) {
	if input == nil || input.Name == "" {
		return nil, fmt.Errorf("CreateVolume: name is required")
	}
	if _, err := os.Stat(s.volumeDir(input.Name)); err == nil {
		return nil, fmt.Errorf("CreateVolume: volume %q already exists", input.Name)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	tags := map[string]string{}
	maps.Copy(tags, input.Tags)

	meta := &volumeMeta{
		Name:      input.Name,
		Owner:     input.Owner,
		Tags:      tags,
		Expires:   input.Expires,
		CreatedAt: time.Now(),
	}
	if err := s.saveVolumeMeta(meta); err != nil {
		return nil, err
	}

	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateVolume) //nolint:errcheck
	}

	return &backends.CreateVolumeOutput{Volume: *volumeFromMeta(meta)}, nil
}

// GetVolumes lists every volume recorded under <configDir>/volumes.
func (s *b) GetVolumes() (backends.VolumeList, error) {
	metas, err := s.listVolumeMetas()
	if err != nil {
		return nil, err
	}
	out := backends.VolumeList{}
	for _, m := range metas {
		out = append(out, volumeFromMeta(m))
	}
	s.volumes = out
	return out, nil
}

// DeleteVolumes removes each volume's on-disk directory (metadata + backing storage).
func (s *b) DeleteVolumes(volumes backends.VolumeList, fw backends.FirewallList, waitDur time.Duration) error {
	if len(volumes) == 0 {
		return nil
	}
	var errs error
	for _, v := range volumes {
		if err := os.RemoveAll(s.volumeDir(v.Name)); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateVolume) //nolint:errcheck
	}
	return errs
}

// mutateVolumeMeta loads each volume's metadata, applies mutate, and saves it back.
func (s *b) mutateVolumeMeta(volumes backends.VolumeList, mutate func(*volumeMeta)) error {
	var errs error
	for _, v := range volumes {
		m, err := loadVolumeMeta(s.volumeMetaFile(v.Name))
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		mutate(m)
		if err := s.saveVolumeMeta(m); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateVolume) //nolint:errcheck
	}
	return errs
}

func (s *b) VolumesAddTags(volumes backends.VolumeList, tags map[string]string, waitDur time.Duration) error {
	return s.mutateVolumeMeta(volumes, func(m *volumeMeta) {
		if m.Tags == nil {
			m.Tags = map[string]string{}
		}
		maps.Copy(m.Tags, tags)
	})
}

func (s *b) VolumesRemoveTags(volumes backends.VolumeList, tagKeys []string, waitDur time.Duration) error {
	return s.mutateVolumeMeta(volumes, func(m *volumeMeta) {
		for _, k := range tagKeys {
			delete(m.Tags, k)
		}
	})
}

// VolumesChangeExpiry sets (or, for a zero expiry, clears) each volume's Expires field,
// mirroring InstancesChangeExpiry's zero-clears semantics from Phase 3.
func (s *b) VolumesChangeExpiry(volumes backends.VolumeList, expiry time.Time) error {
	return s.mutateVolumeMeta(volumes, func(m *volumeMeta) {
		m.Expires = expiry
		if expiry.IsZero() {
			delete(m.Tags, TAG_EXPIRES)
			return
		}
		if m.Tags == nil {
			m.Tags = map[string]string{}
		}
		m.Tags[TAG_EXPIRES] = expiry.Format(time.RFC3339)
	})
}
