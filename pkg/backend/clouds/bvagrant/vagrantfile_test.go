package bvagrant

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "update golden files in testdata/")

const testPubKey = "ssh-ed25519 AAAATESTKEYONLYFORUNITTESTS test@aerolab"

// compareGolden compares got against the golden file testdata/<name>. With -update
// it rewrites the golden file instead of comparing.
func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create testdata dir: %v", err)
		}
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatalf("failed to update golden file %s: %v", path, err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read golden file %s (run with -update to create it): %v", path, err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch for %s:\n--- got ---\n%s\n--- want ---\n%s", name, got, string(want))
	}
}

func TestVagrantfileGoldenSingleNode(t *testing.T) {
	meta := &clusterMeta{
		ClusterName: "mycluster",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-mycluster-1", IP: "192.168.56.2", Box: "bento/ubuntu-22.04"},
		},
	}
	s := &b{configDir: "/etc/aerolab"}
	out, err := s.renderVagrantfile(meta, testPubKey)
	if err != nil {
		t.Fatalf("renderVagrantfile error: %v", err)
	}
	compareGolden(t, "single_node.vf", out)
}

func TestVagrantfileGoldenThreeNode(t *testing.T) {
	meta := &clusterMeta{
		ClusterName: "cluster3",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-cluster3-1", IP: "192.168.56.2", Box: "bento/ubuntu-22.04", CPUs: 2, RAMMB: 2048},
			2: {MachineName: "proj-cluster3-2", IP: "192.168.56.3", Box: "bento/ubuntu-22.04", CPUs: 2, RAMMB: 2048},
			3: {MachineName: "proj-cluster3-3", IP: "192.168.56.4", Box: "bento/ubuntu-22.04", CPUs: 2, RAMMB: 2048},
		},
	}
	s := &b{configDir: "/etc/aerolab"}
	out, err := s.renderVagrantfile(meta, testPubKey)
	if err != nil {
		t.Fatalf("renderVagrantfile error: %v", err)
	}
	compareGolden(t, "three_node.vf", out)

	if !strings.Contains(out, testPubKey) {
		t.Fatalf("expected provisioner block to contain pubkey %q", testPubKey)
	}
	for _, ip := range []string{"192.168.56.2", "192.168.56.3", "192.168.56.4"} {
		if !strings.Contains(out, `ip: "`+ip+`"`) {
			t.Fatalf("expected static IP line for %s in output:\n%s", ip, out)
		}
	}
}

func TestVagrantfileGoldenCustomBox(t *testing.T) {
	meta := &clusterMeta{
		ClusterName: "customboxcluster",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-customboxcluster-1", IP: "192.168.56.2", Box: "custom/special-box", CPUs: 4, RAMMB: 4096},
		},
	}
	s := &b{configDir: "/etc/aerolab"}
	out, err := s.renderVagrantfile(meta, testPubKey)
	if err != nil {
		t.Fatalf("renderVagrantfile error: %v", err)
	}
	compareGolden(t, "custom_box.vf", out)
}

func TestVagrantfileGoldenDisks(t *testing.T) {
	meta := &clusterMeta{
		ClusterName: "diskcluster",
		Nodes: map[int]*nodeMeta{
			1: {
				MachineName: "proj-diskcluster-1",
				IP:          "192.168.56.2",
				Box:         "bento/ubuntu-22.04",
				CPUs:        2,
				RAMMB:       2048,
				Disks:       []string{"data:/mnt/data", "logs:/mnt/logs:ro"},
			},
		},
	}
	s := &b{configDir: "/etc/aerolab"}
	out, err := s.renderVagrantfile(meta, testPubKey)
	if err != nil {
		t.Fatalf("renderVagrantfile error: %v", err)
	}
	compareGolden(t, "disks.vf", out)
}

func TestVagrantfileMalformedDiskEntry(t *testing.T) {
	cases := []string{
		"onlyname",
		"name:/path:ro:extra",
		"name:/path:notro",
		":/path",
		"name:",
		"name:relative/path",
	}
	for _, disk := range cases {
		meta := &clusterMeta{
			ClusterName: "baddisk",
			Nodes: map[int]*nodeMeta{
				1: {MachineName: "proj-baddisk-1", IP: "192.168.56.2", Box: "bento/ubuntu-22.04", Disks: []string{disk}},
			},
		}
		s := &b{configDir: "/etc/aerolab"}
		if _, err := s.renderVagrantfile(meta, testPubKey); err == nil {
			t.Fatalf("expected error for malformed disk entry %q", disk)
		}
	}
}

func TestVagrantfileRegenerationAfterNodeRemoval(t *testing.T) {
	meta := &clusterMeta{
		ClusterName: "shrink",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-shrink-1", IP: "192.168.56.2", Box: "bento/ubuntu-22.04"},
			2: {MachineName: "proj-shrink-2", IP: "192.168.56.3", Box: "bento/ubuntu-22.04"},
			3: {MachineName: "proj-shrink-3", IP: "192.168.56.4", Box: "bento/ubuntu-22.04"},
		},
	}
	s := &b{configDir: "/etc/aerolab"}

	before, err := s.renderVagrantfile(meta, testPubKey)
	if err != nil {
		t.Fatalf("renderVagrantfile error: %v", err)
	}
	if !strings.Contains(before, `"proj-shrink-2"`) {
		t.Fatalf("expected node 2 define block present before removal:\n%s", before)
	}

	delete(meta.Nodes, 2)

	after, err := s.renderVagrantfile(meta, testPubKey)
	if err != nil {
		t.Fatalf("renderVagrantfile error: %v", err)
	}
	if strings.Contains(after, `"proj-shrink-2"`) {
		t.Fatalf("expected node 2 define block removed after deletion:\n%s", after)
	}
	if !strings.Contains(after, `"proj-shrink-1"`) || !strings.Contains(after, `"proj-shrink-3"`) {
		t.Fatalf("expected nodes 1 and 3 still present:\n%s", after)
	}
}

func TestWriteVagrantfile(t *testing.T) {
	dir := t.TempDir()
	s := &b{configDir: dir}
	meta := &clusterMeta{
		ClusterName: "writetest",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-writetest-1", IP: "192.168.56.2", Box: "bento/ubuntu-22.04"},
		},
	}
	if err := s.writeVagrantfile(meta, testPubKey); err != nil {
		t.Fatalf("writeVagrantfile error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(s.clusterDir(meta.ClusterName), "Vagrantfile"))
	if err != nil {
		t.Fatalf("expected Vagrantfile to be written: %v", err)
	}
	if len(content) == 0 {
		t.Fatalf("expected non-empty Vagrantfile content")
	}
	rendered, err := s.renderVagrantfile(meta, testPubKey)
	if err != nil {
		t.Fatalf("renderVagrantfile error: %v", err)
	}
	if string(content) != rendered {
		t.Fatalf("written Vagrantfile does not match renderVagrantfile output")
	}
}
