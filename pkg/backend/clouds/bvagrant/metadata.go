package bvagrant

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/file"
)

// clusterMeta is the on-disk metadata record for a single vagrant cluster.
type clusterMeta struct {
	ClusterName string            `json:"clusterName"`
	ClusterUUID string            `json:"clusterUUID"`
	Owner       string            `json:"owner"`
	Tags        map[string]string `json:"tags"`
	Nodes       map[int]*nodeMeta `json:"nodes"`
}

// nodeMeta is the on-disk metadata record for a single node within a cluster.
type nodeMeta struct {
	MachineName     string            `json:"machineName"`
	IP              string            `json:"ip"`
	Box             string            `json:"box"`
	OSName          string            `json:"osName"`
	OSVersion       string            `json:"osVersion"`
	Arch            string            `json:"arch"`
	CPUs            int               `json:"cpus"`
	RAMMB           int               `json:"ramMB"`
	Disks           []string          `json:"disks"`
	Tags            map[string]string `json:"tags"`
	Expires         time.Time         `json:"expires"`
	Description     string            `json:"description"`
	Owner           string            `json:"owner"`
	CreatedAt       time.Time         `json:"createdAt"`
	TerminateOnStop bool              `json:"terminateOnStop"`
	Provider        string            `json:"provider"`
}

const clusterMetaFileName = "aerolab.json"

// clustersRoot returns the directory under which all per-cluster metadata directories live.
func (s *b) clustersRoot() string {
	return filepath.Join(s.configDir, "clusters")
}

// clusterDir returns the directory holding metadata for a single cluster.
func (s *b) clusterDir(name string) string {
	return filepath.Join(s.clustersRoot(), name)
}

func (s *b) clusterMetaFile(name string) string {
	return filepath.Join(s.clusterDir(name), clusterMetaFileName)
}

// loadClusterMeta loads a cluster's metadata. If the cluster does not exist on disk,
// it returns (nil, nil) — a missing cluster is not an error condition, callers should
// treat a nil result as "no such cluster". A corrupt/unreadable file that exists IS an error.
func (s *b) loadClusterMeta(name string) (*clusterMeta, error) {
	return loadClusterMetaAt(s.clustersRoot(), name)
}

// loadClusterMetaAt is loadClusterMeta parameterized by an explicit clusters-root
// directory, so callers can load metadata belonging to a different project (see
// GetInstances' listAllProjects handling).
func loadClusterMetaAt(clustersRoot, name string) (*clusterMeta, error) {
	raw, err := os.ReadFile(filepath.Join(clustersRoot, name, clusterMetaFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	meta := &clusterMeta{}
	if err := json.Unmarshal(raw, meta); err != nil {
		return nil, fmt.Errorf("corrupt metadata for cluster %q: %w", name, err)
	}
	return meta, nil
}

// saveClusterMeta atomically writes cluster metadata to disk.
func (s *b) saveClusterMeta(meta *clusterMeta) error {
	if meta == nil {
		return fmt.Errorf("cannot save nil cluster metadata")
	}
	if meta.ClusterName == "" {
		return fmt.Errorf("cannot save cluster metadata with empty ClusterName")
	}
	fpath := s.clusterMetaFile(meta.ClusterName)
	if err := os.MkdirAll(filepath.Dir(fpath), 0755); err != nil {
		return err
	}
	return file.StoreJSON(fpath, ".tmp", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644, meta)
}

// deleteClusterMeta removes the entire on-disk directory for a cluster.
func (s *b) deleteClusterMeta(name string) error {
	return os.RemoveAll(s.clusterDir(name))
}

// listClusterMetas scans clustersRoot for all cluster metadata files and loads them.
// Entries that cannot be read (e.g. permission issues, missing file due to a race) are
// skipped with a log warning if a logger is configured; a cluster whose metadata file
// exists but contains corrupt JSON is treated the same way here (skipped + warned),
// since listing is a best-effort scan rather than an explicit single-cluster load.
func (s *b) listClusterMetas() ([]*clusterMeta, error) {
	return s.listClusterMetasAt(s.clustersRoot())
}

// listClusterMetasAt is listClusterMetas parameterized by an explicit clusters-root
// directory (see GetInstances' listAllProjects handling).
func (s *b) listClusterMetasAt(clustersRoot string) ([]*clusterMeta, error) {
	entries, err := os.ReadDir(clustersRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	metas := make([]*clusterMeta, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		meta, err := loadClusterMetaAt(clustersRoot, entry.Name())
		if err != nil {
			if s.log != nil {
				s.log.Warn("skipping unreadable cluster metadata for %q: %v", entry.Name(), err)
			}
			continue
		}
		if meta == nil {
			continue
		}
		metas = append(metas, meta)
	}
	return metas, nil
}

// machineName builds the conventional machine name for a node: "<project>-<cluster>-<nodeNo>".
func (s *b) machineName(project, cluster string, nodeNo int) string {
	return fmt.Sprintf("%s-%s-%d", project, cluster, nodeNo)
}

// lockCluster returns the per-cluster mutex used to serialize metadata reads/writes for a
// given cluster, lazily creating it if needed. Safe to call on a bare &b{} (nil map).
func (s *b) lockCluster(name string) *sync.Mutex {
	s.clusterLocksMu.Lock()
	defer s.clusterLocksMu.Unlock()
	if s.clusterLocks == nil {
		s.clusterLocks = make(map[string]*sync.Mutex)
	}
	mu, ok := s.clusterLocks[name]
	if !ok {
		mu = &sync.Mutex{}
		s.clusterLocks[name] = mu
	}
	return mu
}
