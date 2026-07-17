package bvagrant

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/utils/file"
)

// imageMeta is the on-disk metadata record for a custom image created via CreateImage.
// Unlike boxCatalog's public templates, custom images have no fixed OS/version/arch
// combination baked into aerolab itself, so their descriptive fields are recorded here.
type imageMeta struct {
	Name        string            `json:"name"`
	Box         string            `json:"box"`
	OSName      string            `json:"osName"`
	OSVersion   string            `json:"osVersion"`
	Arch        string            `json:"arch"`
	Description string            `json:"description"`
	Owner       string            `json:"owner"`
	Tags        map[string]string `json:"tags"`
	CreatedAt   time.Time         `json:"createdAt"`
}

func (s *b) imagesDir() string {
	return filepath.Join(s.configDir, "images")
}

func (s *b) imageMetaFile(name string) string {
	return filepath.Join(s.imagesDir(), name+".json")
}

func loadImageMeta(path string) (*imageMeta, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	m := &imageMeta{}
	if err := json.Unmarshal(raw, m); err != nil {
		return nil, fmt.Errorf("corrupt image metadata %q: %w", path, err)
	}
	return m, nil
}

func (s *b) saveImageMeta(m *imageMeta) error {
	if err := os.MkdirAll(s.imagesDir(), 0755); err != nil {
		return err
	}
	return file.StoreJSON(s.imageMetaFile(m.Name), ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, m)
}

// listImageMetas scans imagesDir for all custom image metadata files. A file that can't
// be read/parsed is skipped with a log warning, matching listClusterMetas' best-effort
// scanning behavior.
func (s *b) listImageMetas() ([]*imageMeta, error) {
	entries, err := os.ReadDir(s.imagesDir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var metas []*imageMeta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		m, err := loadImageMeta(filepath.Join(s.imagesDir(), e.Name()))
		if err != nil {
			if s.log != nil {
				s.log.Warn("skipping unreadable image metadata %q: %v", e.Name(), err)
			}
			continue
		}
		metas = append(metas, m)
	}
	return metas, nil
}

// customImageBoxName builds the vagrant box name a packaged custom image is registered
// under: "aerolab-<project>-<name>".
func customImageBoxName(project, name string) string {
	return fmt.Sprintf("aerolab-%s-%s", project, name)
}

// GetImages synthesizes one public template Image per boxCatalog {distro,version,arch}
// combination (InAccount reflects whether the box is present in `vagrant box list`),
// plus one Image per custom image recorded under <configDir>/images/*.json.
func (s *b) GetImages() (backends.ImageList, error) {
	var out backends.ImageList

	boxNames := map[string]bool{}
	if s.runner != nil {
		boxes, err := s.runner.BoxList()
		if err != nil {
			return nil, err
		}
		for _, box := range boxes {
			boxNames[box.Name] = true
		}
	}

	for _, entry := range boxCatalog {
		for _, archStr := range entry.Archs {
			var arch backends.Architecture
			if err := arch.FromString(archStr); err != nil {
				return nil, err
			}
			out = append(out, &backends.Image{
				BackendType:  backends.BackendTypeVagrant,
				Name:         entry.Box,
				Description:  fmt.Sprintf("Default image for %s %s %s", entry.Distro, entry.Version, archStr),
				ImageId:      entry.Box,
				ZoneName:     "local",
				ZoneID:       "local",
				Owner:        "",
				Tags:         map[string]string{},
				Architecture: arch,
				Public:       true,
				State:        backends.VolumeStateAvailable,
				OSName:       entry.Distro,
				OSVersion:    entry.Version,
				InAccount:    boxNames[entry.Box],
				Username:     "root",
			})
		}
	}

	metas, err := s.listImageMetas()
	if err != nil {
		return nil, err
	}
	for _, m := range metas {
		var arch backends.Architecture
		arch.FromString(m.Arch) //nolint:errcheck
		out = append(out, &backends.Image{
			BackendType:  backends.BackendTypeVagrant,
			Name:         m.Name,
			Description:  m.Description,
			ImageId:      m.Box,
			ZoneName:     "local",
			ZoneID:       "local",
			CreationTime: m.CreatedAt,
			Owner:        m.Owner,
			Tags:         m.Tags,
			Architecture: arch,
			Public:       false,
			State:        backends.VolumeStateAvailable,
			OSName:       m.OSName,
			OSVersion:    m.OSVersion,
			InAccount:    true,
			Username:     "root",
		})
	}

	s.images = out
	return out, nil
}

// CreateImage packages a (halted) source instance into a new vagrant box and records
// custom image metadata for it. If the source instance is running, it is halted first
// and restarted afterward (packaging requires the VM to be stopped); any failure after
// the halt still attempts to restart the source before returning.
func (s *b) CreateImage(input *backends.CreateImageInput, waitDur time.Duration) (*backends.CreateImageOutput, error) {
	if input == nil || input.Instance == nil {
		return nil, fmt.Errorf("CreateImage: source instance is required")
	}
	detail, ok := input.Instance.BackendSpecific.(*InstanceDetail)
	if !ok || detail == nil {
		return nil, fmt.Errorf("CreateImage: instance %q has no vagrant backend detail", input.Instance.InstanceID)
	}

	wasRunning := input.Instance.InstanceState == backends.LifeCycleStateRunning
	if wasRunning {
		if err := s.runner.Halt(detail.ClusterDir, []string{detail.MachineName}); err != nil {
			return nil, fmt.Errorf("CreateImage: failed to halt source instance before packaging: %w", err)
		}
	}

	if err := os.MkdirAll(s.imagesDir(), 0755); err != nil {
		return nil, err
	}
	tmpPath := filepath.Join(s.imagesDir(), ".tmp-"+input.Name+".box")

	if err := s.runner.Package(detail.ClusterDir, detail.MachineName, tmpPath); err != nil {
		if wasRunning {
			_ = s.runner.Up(detail.ClusterDir, []string{detail.MachineName}, detail.Provider, false)
		}
		return nil, fmt.Errorf("CreateImage: vagrant package failed (note: `vagrant package` only supports virtualbox-family providers): %w", err)
	}

	boxName := customImageBoxName(s.project, input.Name)
	addErr := s.runner.BoxAdd(boxName, tmpPath)
	os.Remove(tmpPath) //nolint:errcheck
	if addErr != nil {
		if wasRunning {
			_ = s.runner.Up(detail.ClusterDir, []string{detail.MachineName}, detail.Provider, false)
		}
		return nil, fmt.Errorf("CreateImage: failed to register packaged box: %w", addErr)
	}

	if wasRunning {
		if err := s.runner.Up(detail.ClusterDir, []string{detail.MachineName}, detail.Provider, false); err != nil {
			return nil, fmt.Errorf("CreateImage: failed to restart source instance after packaging: %w", err)
		}
	}

	osName := input.OSName
	if osName == "" {
		osName = input.Instance.OperatingSystem.Name
	}
	osVersion := input.OSVersion
	if osVersion == "" {
		osVersion = input.Instance.OperatingSystem.Version
	}

	tags := map[string]string{}
	maps.Copy(tags, input.Tags)

	meta := &imageMeta{
		Name:        input.Name,
		Box:         boxName,
		OSName:      osName,
		OSVersion:   osVersion,
		Arch:        input.Instance.Architecture.String(),
		Description: input.Description,
		Owner:       input.Owner,
		Tags:        tags,
		CreatedAt:   time.Now(),
	}
	if err := s.saveImageMeta(meta); err != nil {
		return nil, err
	}

	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateImage) //nolint:errcheck
	}

	return &backends.CreateImageOutput{
		Image: &backends.Image{
			BackendType:  backends.BackendTypeVagrant,
			Name:         input.Name,
			Description:  input.Description,
			ImageId:      boxName,
			ZoneName:     "local",
			ZoneID:       "local",
			CreationTime: meta.CreatedAt,
			Owner:        input.Owner,
			Tags:         tags,
			Architecture: input.Instance.Architecture,
			Public:       false,
			State:        backends.VolumeStateAvailable,
			OSName:       osName,
			OSVersion:    osVersion,
			InAccount:    true,
			Username:     "root",
		},
	}, nil
}

// ImagesDelete deregisters each image's box from vagrant and, for non-public (custom)
// images, removes its metadata file.
func (s *b) ImagesDelete(images backends.ImageList, waitDur time.Duration) error {
	if len(images) == 0 {
		return nil
	}
	var errs error
	for _, img := range images {
		if err := s.runner.BoxRemove(img.ImageId); err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		if !img.Public {
			if err := os.Remove(s.imageMetaFile(img.Name)); err != nil && !os.IsNotExist(err) {
				errs = errors.Join(errs, err)
			}
		}
	}
	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateImage) //nolint:errcheck
	}
	return errs
}

// mutateImageTags loads each image's metadata file, applies mutate, and saves it back.
// Images with no metadata file (e.g. public templates) are skipped rather than erroring.
func (s *b) mutateImageTags(images backends.ImageList, mutate func(*imageMeta)) error {
	var errs error
	for _, img := range images {
		m, err := loadImageMeta(s.imageMetaFile(img.Name))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			errs = errors.Join(errs, err)
			continue
		}
		mutate(m)
		if err := s.saveImageMeta(m); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

func (s *b) ImagesAddTags(images backends.ImageList, tags map[string]string) error {
	return s.mutateImageTags(images, func(m *imageMeta) {
		if m.Tags == nil {
			m.Tags = map[string]string{}
		}
		maps.Copy(m.Tags, tags)
	})
}

func (s *b) ImagesRemoveTags(images backends.ImageList, tagKeys []string) error {
	return s.mutateImageTags(images, func(m *imageMeta) {
		for _, k := range tagKeys {
			delete(m.Tags, k)
		}
	})
}
