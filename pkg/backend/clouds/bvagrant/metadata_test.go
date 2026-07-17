package bvagrant

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestBackend(t *testing.T) *b {
	t.Helper()
	return &b{configDir: t.TempDir()}
}

func TestMetadataRoundTrip(t *testing.T) {
	s := newTestBackend(t)

	meta := &clusterMeta{
		ClusterName: "mycluster",
		ClusterUUID: "uuid-1234",
		Owner:       "jdoty",
		Tags:        map[string]string{"env": "test"},
		Nodes: map[int]*nodeMeta{
			1: {
				MachineName: "proj-mycluster-1",
				IP:          "192.168.56.10",
				Box:         "bento/ubuntu-24.04",
				OSName:      "ubuntu",
				OSVersion:   "24.04",
				Arch:        "amd64",
				CPUs:        2,
				RAMMB:       2048,
				Disks:       []string{"/dev/sdb"},
				Tags:        map[string]string{"role": "server"},
				Expires:     time.Now().Add(time.Hour).Truncate(time.Second).UTC(),
				Description: "test node",
				Owner:       "jdoty",
				CreatedAt:   time.Now().Truncate(time.Second).UTC(),
			},
		},
	}

	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}

	loaded, err := s.loadClusterMeta("mycluster")
	if err != nil {
		t.Fatalf("loadClusterMeta: %v", err)
	}
	if loaded == nil {
		t.Fatalf("loadClusterMeta returned nil for existing cluster")
	}

	if loaded.ClusterName != meta.ClusterName || loaded.ClusterUUID != meta.ClusterUUID || loaded.Owner != meta.Owner {
		t.Fatalf("cluster fields mismatch: got %+v", loaded)
	}
	if loaded.Tags["env"] != "test" {
		t.Fatalf("tags mismatch: got %+v", loaded.Tags)
	}
	n, ok := loaded.Nodes[1]
	if !ok {
		t.Fatalf("node 1 missing after round trip")
	}
	if n.MachineName != "proj-mycluster-1" || n.IP != "192.168.56.10" || n.Box != "bento/ubuntu-24.04" {
		t.Fatalf("node fields mismatch: got %+v", n)
	}
	if !n.Expires.Equal(meta.Nodes[1].Expires) {
		t.Fatalf("expires mismatch: got %v want %v", n.Expires, meta.Nodes[1].Expires)
	}
}

func TestMetadataMissingClusterReturnsNilNoError(t *testing.T) {
	s := newTestBackend(t)

	loaded, err := s.loadClusterMeta("doesnotexist")
	if err != nil {
		t.Fatalf("expected nil error for missing cluster, got %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil clusterMeta for missing cluster, got %+v", loaded)
	}
}

func TestMetadataNodeAddRemove(t *testing.T) {
	s := newTestBackend(t)

	meta := &clusterMeta{
		ClusterName: "addremove",
		Nodes:       map[int]*nodeMeta{},
	}
	meta.Nodes[1] = &nodeMeta{MachineName: "addremove-1", IP: "192.168.56.11"}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}

	loaded, err := s.loadClusterMeta("addremove")
	if err != nil || loaded == nil {
		t.Fatalf("loadClusterMeta failed: %v %+v", err, loaded)
	}
	loaded.Nodes[2] = &nodeMeta{MachineName: "addremove-2", IP: "192.168.56.12"}
	if err := s.saveClusterMeta(loaded); err != nil {
		t.Fatalf("saveClusterMeta (add node): %v", err)
	}

	loaded2, err := s.loadClusterMeta("addremove")
	if err != nil || loaded2 == nil {
		t.Fatalf("loadClusterMeta after add failed: %v %+v", err, loaded2)
	}
	if len(loaded2.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(loaded2.Nodes))
	}

	delete(loaded2.Nodes, 1)
	if err := s.saveClusterMeta(loaded2); err != nil {
		t.Fatalf("saveClusterMeta (remove node): %v", err)
	}

	loaded3, err := s.loadClusterMeta("addremove")
	if err != nil || loaded3 == nil {
		t.Fatalf("loadClusterMeta after remove failed: %v %+v", err, loaded3)
	}
	if len(loaded3.Nodes) != 1 {
		t.Fatalf("expected 1 node after removal, got %d", len(loaded3.Nodes))
	}
	if _, ok := loaded3.Nodes[2]; !ok {
		t.Fatalf("expected node 2 to remain")
	}
}

func TestMetadataTagAddRemove(t *testing.T) {
	s := newTestBackend(t)

	meta := &clusterMeta{
		ClusterName: "tagtest",
		Tags:        map[string]string{},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}

	loaded, err := s.loadClusterMeta("tagtest")
	if err != nil || loaded == nil {
		t.Fatalf("loadClusterMeta failed: %v", err)
	}
	if loaded.Tags == nil {
		loaded.Tags = map[string]string{}
	}
	loaded.Tags["foo"] = "bar"
	if err := s.saveClusterMeta(loaded); err != nil {
		t.Fatalf("saveClusterMeta (add tag): %v", err)
	}

	loaded2, err := s.loadClusterMeta("tagtest")
	if err != nil || loaded2 == nil || loaded2.Tags["foo"] != "bar" {
		t.Fatalf("expected tag foo=bar, got %+v (err=%v)", loaded2, err)
	}

	delete(loaded2.Tags, "foo")
	if err := s.saveClusterMeta(loaded2); err != nil {
		t.Fatalf("saveClusterMeta (remove tag): %v", err)
	}

	loaded3, err := s.loadClusterMeta("tagtest")
	if err != nil || loaded3 == nil {
		t.Fatalf("loadClusterMeta failed: %v", err)
	}
	if _, ok := loaded3.Tags["foo"]; ok {
		t.Fatalf("expected tag foo to be removed, got %+v", loaded3.Tags)
	}
}

func TestMetadataExpirySetClear(t *testing.T) {
	s := newTestBackend(t)

	exp := time.Now().Add(2 * time.Hour).Truncate(time.Second).UTC()
	meta := &clusterMeta{
		ClusterName: "expirytest",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "expirytest-1", Expires: exp},
		},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}

	loaded, err := s.loadClusterMeta("expirytest")
	if err != nil || loaded == nil {
		t.Fatalf("loadClusterMeta failed: %v", err)
	}
	if !loaded.Nodes[1].Expires.Equal(exp) {
		t.Fatalf("expiry not set: got %v want %v", loaded.Nodes[1].Expires, exp)
	}

	loaded.Nodes[1].Expires = time.Time{}
	if err := s.saveClusterMeta(loaded); err != nil {
		t.Fatalf("saveClusterMeta (clear expiry): %v", err)
	}

	loaded2, err := s.loadClusterMeta("expirytest")
	if err != nil || loaded2 == nil {
		t.Fatalf("loadClusterMeta failed: %v", err)
	}
	if !loaded2.Nodes[1].Expires.IsZero() {
		t.Fatalf("expected expiry cleared, got %v", loaded2.Nodes[1].Expires)
	}
}

func TestMetadataCorruptFileErrors(t *testing.T) {
	s := newTestBackend(t)

	dir := s.clusterDir("corrupt")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "aerolab.json"), []byte("{not valid json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	_, err := s.loadClusterMeta("corrupt")
	if err == nil {
		t.Fatalf("expected error loading corrupt metadata file, got nil")
	}
}

func TestMetadataDeleteCluster(t *testing.T) {
	s := newTestBackend(t)

	meta := &clusterMeta{ClusterName: "deleteme"}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	if err := s.deleteClusterMeta("deleteme"); err != nil {
		t.Fatalf("deleteClusterMeta: %v", err)
	}
	loaded, err := s.loadClusterMeta("deleteme")
	if err != nil {
		t.Fatalf("loadClusterMeta after delete: %v", err)
	}
	if loaded != nil {
		t.Fatalf("expected nil after delete, got %+v", loaded)
	}
	if _, err := os.Stat(s.clusterDir("deleteme")); !os.IsNotExist(err) {
		t.Fatalf("expected cluster dir removed, stat err=%v", err)
	}
}

func TestMetadataListClusterMetas(t *testing.T) {
	s := newTestBackend(t)

	for _, name := range []string{"c1", "c2", "c3"} {
		if err := s.saveClusterMeta(&clusterMeta{ClusterName: name}); err != nil {
			t.Fatalf("saveClusterMeta(%s): %v", name, err)
		}
	}

	list, err := s.listClusterMetas()
	if err != nil {
		t.Fatalf("listClusterMetas: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 clusters, got %d", len(list))
	}
}

func TestMetadataMachineName(t *testing.T) {
	s := newTestBackend(t)
	name := s.machineName("proj", "clus", 3)
	if name != "proj-clus-3" {
		t.Fatalf("machineName mismatch: got %s", name)
	}
}

func TestMetadataConcurrentAccessUnderLock(t *testing.T) {
	s := newTestBackend(t)
	clusterName := "concurrent"
	if err := s.saveClusterMeta(&clusterMeta{ClusterName: clusterName, Nodes: map[int]*nodeMeta{}}); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}

	const n = 20
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mu := s.lockCluster(clusterName)
			mu.Lock()
			defer mu.Unlock()

			meta, err := s.loadClusterMeta(clusterName)
			if err != nil || meta == nil {
				t.Errorf("loadClusterMeta in goroutine %d: %v %+v", i, err, meta)
				return
			}
			if meta.Nodes == nil {
				meta.Nodes = map[int]*nodeMeta{}
			}
			meta.Nodes[i] = &nodeMeta{MachineName: s.machineName("proj", clusterName, i)}
			if err := s.saveClusterMeta(meta); err != nil {
				t.Errorf("saveClusterMeta in goroutine %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()

	final, err := s.loadClusterMeta(clusterName)
	if err != nil || final == nil {
		t.Fatalf("final loadClusterMeta: %v %+v", err, final)
	}
	if len(final.Nodes) != n {
		t.Fatalf("expected %d nodes after concurrent writes, got %d", n, len(final.Nodes))
	}
}
