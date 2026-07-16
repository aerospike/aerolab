package bvagrant

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds"
	"github.com/rglonek/logger"
)

// newVagrantTestBackend builds a *b wired to a fakeRunner and temp directories,
// ready for CreateInstances/GetInstances/etc. tests. lookPath is stubbed to
// succeed for everything (so Preflight's provider detection finds virtualbox).
func newVagrantTestBackend(t *testing.T) (*b, *fakeRunner) {
	t.Helper()
	fr := &fakeRunner{
		versionResult: "2.4.9",
	}
	s := &b{
		configDir:    t.TempDir(),
		sshKeysDir:   t.TempDir(),
		project:      "proj",
		credentials:  &clouds.VAGRANT{Subnet: "192.168.56.0/24"},
		runner:       fr,
		log:          logger.NewLogger(),
		lookPath:     func(name string) (string, error) { return "/usr/bin/" + name, nil },
		sshReadyPoll: func(instances backends.InstanceList, waitDur time.Duration) error { return nil },
	}
	return s, fr
}

// writeValidPreflightCache pre-seeds a passing preflight.json so CreateInstances'
// internal Preflight(false) call doesn't need to hit the fakeRunner/lookPath path.
func writeValidPreflightCache(t *testing.T, s *b) {
	t.Helper()
	res := &preflightResult{
		CheckedAt:      time.Now(),
		VagrantVersion: "2.4.9",
		Providers:      []string{"virtualbox"},
		OK:             true,
	}
	if err := s.savePreflightCache(res); err != nil {
		t.Fatalf("savePreflightCache: %v", err)
	}
}

func testImage() *backends.Image {
	return &backends.Image{
		OSName:       "ubuntu",
		OSVersion:    "22.04",
		Architecture: backends.ArchitectureX8664,
	}
}

// ---- mapVagrantState ----

func TestMapVagrantState(t *testing.T) {
	cases := []struct {
		raw  string
		want backends.LifeCycleState
	}{
		{"running", backends.LifeCycleStateRunning},
		{"poweroff", backends.LifeCycleStateStopped},
		{"saved", backends.LifeCycleStateStopped},
		{"aborted", backends.LifeCycleStateStopped},
		{"some-unknown-state", backends.LifeCycleStateUnknown},
	}
	for _, c := range cases {
		got := mapVagrantState(c.raw)
		if got != c.want {
			t.Errorf("mapVagrantState(%q) = %v, want %v", c.raw, got, c.want)
		}
	}
}

// ---- CreateInstances: box resolution ----

func TestCreateInstancesBoxOverrideWins(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	input := &backends.CreateInstanceInput{
		ClusterName: "c1",
		Nodes:       1,
		BackendType: backends.BackendTypeVagrant,
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{
				Image: testImage(),
				Box:   "custom/box",
			},
		},
	}
	fr.statusResult = map[string]string{"proj-c1-1": "running"}

	out, err := s.CreateInstances(input, 0)
	if err != nil {
		t.Fatalf("CreateInstances: %v", err)
	}
	if len(out.Instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(out.Instances))
	}

	meta, err := s.loadClusterMeta("c1")
	if err != nil || meta == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta)
	}
	if meta.Nodes[1].Box != "custom/box" {
		t.Fatalf("expected box override to win, got %q", meta.Nodes[1].Box)
	}
}

// A custom image's ImageId IS its vagrant box name; box resolution must use it
// instead of re-deriving the base box from the catalog (regression: cluster
// create built nodes from the pristine bento box instead of the aerospike
// template image).
func TestCreateInstancesBoxFromImageID(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	img := testImage()
	img.ImageId = "aerolab-proj-mytemplate"
	input := &backends.CreateInstanceInput{
		ClusterName: "cid",
		Nodes:       1,
		BackendType: backends.BackendTypeVagrant,
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{
				Image: img,
			},
		},
	}
	fr.statusResult = map[string]string{"proj-cid-1": "running"}

	if _, err := s.CreateInstances(input, 0); err != nil {
		t.Fatalf("CreateInstances: %v", err)
	}
	meta, err := s.loadClusterMeta("cid")
	if err != nil || meta == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta)
	}
	if meta.Nodes[1].Box != "aerolab-proj-mytemplate" {
		t.Fatalf("expected box from ImageId, got %q", meta.Nodes[1].Box)
	}
}

func TestCreateInstancesBoxFromImage(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	input := &backends.CreateInstanceInput{
		ClusterName: "c2",
		Nodes:       1,
		BackendType: backends.BackendTypeVagrant,
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{
				Image: testImage(),
			},
		},
	}
	fr.statusResult = map[string]string{"proj-c2-1": "running"}

	_, err := s.CreateInstances(input, 0)
	if err != nil {
		t.Fatalf("CreateInstances: %v", err)
	}
	meta, err := s.loadClusterMeta("c2")
	if err != nil || meta == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta)
	}
	if meta.Nodes[1].Box != "bento/ubuntu-22.04" {
		t.Fatalf("expected box resolved from image, got %q", meta.Nodes[1].Box)
	}
}

func TestCreateInstancesUnmappableImageFailsBeforeUp(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	input := &backends.CreateInstanceInput{
		ClusterName: "c3",
		Nodes:       1,
		BackendType: backends.BackendTypeVagrant,
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{
				Image: &backends.Image{OSName: "plan9", OSVersion: "4", Architecture: backends.ArchitectureX8664},
			},
		},
	}

	_, err := s.CreateInstances(input, 0)
	if err == nil {
		t.Fatalf("expected error for unmappable image")
	}
	for _, c := range fr.callsSnapshot() {
		if c.method == "Up" {
			t.Fatalf("expected no Up call, got one: %+v", c)
		}
	}
}

// ---- CreateInstances: keypair generation ----

func TestCreateInstancesGeneratesKeypair(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	input := &backends.CreateInstanceInput{
		ClusterName: "keytest",
		Nodes:       1,
		BackendType: backends.BackendTypeVagrant,
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{Image: testImage()},
		},
	}
	fr.statusResult = map[string]string{"proj-keytest-1": "running"}

	_, err := s.CreateInstances(input, 0)
	if err != nil {
		t.Fatalf("CreateInstances: %v", err)
	}

	privPath := filepath.Join(s.sshKeysDir, "proj")
	pubPath := privPath + ".pub"
	if _, err := os.Stat(privPath); err != nil {
		t.Fatalf("expected private key at %s: %v", privPath, err)
	}
	pubBytes, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("expected public key at %s: %v", pubPath, err)
	}

	dir := s.clusterDir("keytest")
	vf, err := os.ReadFile(filepath.Join(dir, "Vagrantfile"))
	if err != nil {
		t.Fatalf("reading Vagrantfile: %v", err)
	}
	if !strings.Contains(string(vf), strings.TrimSpace(string(pubBytes))) {
		t.Fatalf("expected Vagrantfile to embed the generated pubkey")
	}

	// A second CreateInstances call must reuse the same keypair, not regenerate it.
	priv1, _ := os.ReadFile(privPath)
	input.ClusterName = "keytest2"
	fr.statusResult = map[string]string{"proj-keytest2-1": "running"}
	if _, err := s.CreateInstances(input, 0); err != nil {
		t.Fatalf("second CreateInstances: %v", err)
	}
	priv2, _ := os.ReadFile(privPath)
	if string(priv1) != string(priv2) {
		t.Fatalf("expected private key to be reused, not regenerated")
	}
}

// ---- CreateInstances: node numbering, tags, Up args ----

func TestCreateInstancesNodeNumberingContinuesAndUpArgs(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	input := &backends.CreateInstanceInput{
		ClusterName: "grow",
		Nodes:       2,
		BackendType: backends.BackendTypeVagrant,
		Owner:       "jdoty",
		Tags:        map[string]string{"custom": "tag1"},
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{Image: testImage()},
		},
	}
	fr.statusResult = map[string]string{
		"proj-grow-1": "running",
		"proj-grow-2": "running",
	}

	out, err := s.CreateInstances(input, 0)
	if err != nil {
		t.Fatalf("CreateInstances: %v", err)
	}
	if len(out.Instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(out.Instances))
	}

	calls := fr.callsSnapshot()
	var upArgs string
	for _, c := range calls {
		if c.method == "Up" {
			upArgs = c.args
		}
	}
	if !strings.Contains(upArgs, "proj-grow-1") || !strings.Contains(upArgs, "proj-grow-2") {
		t.Fatalf("expected Up call with new machines, got: %s", upArgs)
	}

	meta1, err := s.loadClusterMeta("grow")
	if err != nil || meta1 == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta1)
	}
	clusterUUID := meta1.ClusterUUID
	if len(meta1.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(meta1.Nodes))
	}
	if meta1.Nodes[1].Tags["custom"] != "tag1" || meta1.Nodes[1].Tags[TAG_OWNER] != "jdoty" {
		t.Fatalf("expected merged tags, got %+v", meta1.Nodes[1].Tags)
	}

	// Grow the cluster by 2 more nodes.
	fr.statusResult = map[string]string{
		"proj-grow-1": "running",
		"proj-grow-2": "running",
		"proj-grow-3": "running",
		"proj-grow-4": "running",
	}
	input.Nodes = 2
	out2, err := s.CreateInstances(input, 0)
	if err != nil {
		t.Fatalf("second CreateInstances: %v", err)
	}
	if len(out2.Instances) != 2 {
		t.Fatalf("expected 2 new instances from second call, got %d", len(out2.Instances))
	}

	calls = fr.callsSnapshot()
	var secondUpArgs string
	upCount := 0
	for _, c := range calls {
		if c.method == "Up" {
			upCount++
			secondUpArgs = c.args
		}
	}
	if upCount != 2 {
		t.Fatalf("expected 2 Up calls total, got %d", upCount)
	}
	if strings.Contains(secondUpArgs, "proj-grow-1") || strings.Contains(secondUpArgs, "proj-grow-2") {
		t.Fatalf("expected second Up call to contain ONLY new machines, got: %s", secondUpArgs)
	}
	if !strings.Contains(secondUpArgs, "proj-grow-3") || !strings.Contains(secondUpArgs, "proj-grow-4") {
		t.Fatalf("expected second Up call to contain the new machines, got: %s", secondUpArgs)
	}

	meta2, err := s.loadClusterMeta("grow")
	if err != nil || meta2 == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta2)
	}
	if len(meta2.Nodes) != 4 {
		t.Fatalf("expected 4 nodes total, got %d", len(meta2.Nodes))
	}
	if meta2.ClusterUUID != clusterUUID {
		t.Fatalf("expected ClusterUUID to be reused, got %q want %q", meta2.ClusterUUID, clusterUUID)
	}
	if _, ok := meta2.Nodes[3]; !ok {
		t.Fatalf("expected node 3 to exist")
	}
	if _, ok := meta2.Nodes[4]; !ok {
		t.Fatalf("expected node 4 to exist")
	}
}

// ---- CreateInstances: ghost-node reconciliation & failed-up rollback ----

// TestPruneGhostNodesRemovesNotCreated verifies that a node whose VM reports
// not_created is dropped from metadata (in-memory and on-disk) and no longer
// appears in the regenerated Vagrantfile, while live nodes are kept.
func TestPruneGhostNodesRemovesNotCreated(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	pubKey, err := s.ensureProjectKeypair()
	if err != nil {
		t.Fatalf("ensureProjectKeypair: %v", err)
	}
	meta := &clusterMeta{ClusterName: "gh", ClusterUUID: "uuid-gh", Nodes: map[int]*nodeMeta{}}
	for _, no := range []int{1, 2, 3} {
		meta.Nodes[no] = &nodeMeta{
			MachineName: s.machineName(s.project, "gh", no),
			IP:          fmt.Sprintf("192.168.56.%d", no+1),
			Box:         "bento/ubuntu-24.04",
		}
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	if err := s.writeVagrantfile(meta, pubKey); err != nil {
		t.Fatalf("writeVagrantfile: %v", err)
	}
	// node 2's VM was destroyed outside aerolab.
	fr.statusResult = map[string]string{
		"proj-gh-1": "running",
		"proj-gh-2": "not_created",
		"proj-gh-3": "poweroff",
	}

	if err := s.pruneGhostNodes(meta, pubKey); err != nil {
		t.Fatalf("pruneGhostNodes: %v", err)
	}

	if _, ok := meta.Nodes[2]; ok {
		t.Fatalf("expected node 2 pruned from in-memory meta, got %+v", meta.Nodes)
	}
	if len(meta.Nodes) != 2 {
		t.Fatalf("expected 2 nodes after prune, got %d", len(meta.Nodes))
	}
	reloaded, err := s.loadClusterMeta("gh")
	if err != nil || reloaded == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, reloaded)
	}
	if _, ok := reloaded.Nodes[2]; ok {
		t.Fatalf("expected node 2 pruned on disk, got %+v", reloaded.Nodes)
	}
	vf, err := os.ReadFile(filepath.Join(s.clusterDir("gh"), "Vagrantfile"))
	if err != nil {
		t.Fatalf("read Vagrantfile: %v", err)
	}
	if strings.Contains(string(vf), "proj-gh-2") {
		t.Fatalf("expected Vagrantfile to no longer define proj-gh-2:\n%s", vf)
	}
}

// TestPruneGhostNodesAllGhostsDeletesCluster verifies that when every node is a
// ghost, the whole cluster metadata directory is removed.
func TestPruneGhostNodesAllGhostsDeletesCluster(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	pubKey, err := s.ensureProjectKeypair()
	if err != nil {
		t.Fatalf("ensureProjectKeypair: %v", err)
	}
	meta := &clusterMeta{ClusterName: "allgh", ClusterUUID: "u", Nodes: map[int]*nodeMeta{
		1: {MachineName: "proj-allgh-1", Box: "b"},
		2: {MachineName: "proj-allgh-2", Box: "b"},
	}}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{"proj-allgh-1": "not_created", "proj-allgh-2": "not_created"}

	if err := s.pruneGhostNodes(meta, pubKey); err != nil {
		t.Fatalf("pruneGhostNodes: %v", err)
	}
	reloaded, err := s.loadClusterMeta("allgh")
	if err != nil {
		t.Fatalf("loadClusterMeta: %v", err)
	}
	if reloaded != nil {
		t.Fatalf("expected cluster metadata deleted, got %+v", reloaded)
	}
}

// TestCreateInstancesGhostNodesDoNotInflateNumbering reproduces the original bug:
// after both VMs of a 2-node cluster are destroyed outside aerolab, a fresh create
// must restart numbering at 1,2 (the ghosts are reconciled away) rather than grow
// to 3,4.
func TestCreateInstancesGhostNodesDoNotInflateNumbering(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	input := &backends.CreateInstanceInput{
		ClusterName: "reset",
		Nodes:       2,
		BackendType: backends.BackendTypeVagrant,
		Owner:       "jdoty",
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{Image: testImage()},
		},
	}
	fr.statusResult = map[string]string{"proj-reset-1": "running", "proj-reset-2": "running"}
	if _, err := s.CreateInstances(input, 0); err != nil {
		t.Fatalf("first CreateInstances: %v", err)
	}

	// The old VMs report not_created at reconciliation time (call 1), while the
	// re-created machines report running by the time GetInstances runs (call 2).
	var statusCalls int
	fr.statusFunc = func(dir string) (map[string]string, error) {
		statusCalls++
		if statusCalls == 1 {
			return map[string]string{"proj-reset-1": "not_created", "proj-reset-2": "not_created"}, nil
		}
		return map[string]string{"proj-reset-1": "running", "proj-reset-2": "running"}, nil
	}

	out, err := s.CreateInstances(input, 0)
	if err != nil {
		t.Fatalf("second CreateInstances: %v", err)
	}
	if len(out.Instances) != 2 {
		t.Fatalf("expected 2 instances, got %d", len(out.Instances))
	}
	for _, inst := range out.Instances {
		if inst.NodeNo != 1 && inst.NodeNo != 2 {
			t.Fatalf("expected node numbers 1,2 after ghost prune, got %d", inst.NodeNo)
		}
	}
	meta, err := s.loadClusterMeta("reset")
	if err != nil || meta == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta)
	}
	if len(meta.Nodes) != 2 {
		t.Fatalf("expected 2 nodes after ghost prune + recreate, got %d: %+v", len(meta.Nodes), meta.Nodes)
	}
	if _, ok := meta.Nodes[3]; ok {
		t.Fatalf("did not expect node 3 to exist after ghost prune")
	}
}

// TestCreateInstancesRollsBackOnUpFailure verifies that a failed `vagrant up` on a
// brand-new cluster destroys the half-created machines and removes the metadata, so
// no ghost is left to inflate future node numbering.
func TestCreateInstancesRollsBackOnUpFailure(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)
	fr.errs = map[string]error{"Up": fmt.Errorf("vagrant error: VMBootTimeout")}

	input := &backends.CreateInstanceInput{
		ClusterName: "failup",
		Nodes:       2,
		BackendType: backends.BackendTypeVagrant,
		Owner:       "jdoty",
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{Image: testImage()},
		},
	}
	if _, err := s.CreateInstances(input, 0); err == nil {
		t.Fatalf("expected CreateInstances to fail on Up error")
	}

	meta, err := s.loadClusterMeta("failup")
	if err != nil {
		t.Fatalf("loadClusterMeta: %v", err)
	}
	if meta != nil {
		t.Fatalf("expected metadata removed after failed create, got %+v", meta)
	}
	var destroyArgs string
	for _, c := range fr.callsSnapshot() {
		if c.method == "Destroy" {
			destroyArgs = c.args
		}
	}
	if !strings.Contains(destroyArgs, "proj-failup-1") || !strings.Contains(destroyArgs, "proj-failup-2") {
		t.Fatalf("expected rollback Destroy of the new machines, got: %q", destroyArgs)
	}
}

// TestCreateInstancesRollbackPreservesExistingNodes verifies that when a grow's
// `vagrant up` fails, only the newly-allocated node is rolled back; the pre-existing
// node's metadata is preserved.
func TestCreateInstancesRollbackPreservesExistingNodes(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	input := &backends.CreateInstanceInput{
		ClusterName: "grow2",
		Nodes:       1,
		BackendType: backends.BackendTypeVagrant,
		Owner:       "jdoty",
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{Image: testImage()},
		},
	}
	fr.statusResult = map[string]string{"proj-grow2-1": "running"}
	if _, err := s.CreateInstances(input, 0); err != nil {
		t.Fatalf("first CreateInstances: %v", err)
	}

	fr.errs = map[string]error{"Up": fmt.Errorf("vagrant error: VMBootTimeout")}
	input.Nodes = 1
	if _, err := s.CreateInstances(input, 0); err == nil {
		t.Fatalf("expected grow to fail on Up error")
	}

	meta, err := s.loadClusterMeta("grow2")
	if err != nil || meta == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta)
	}
	if len(meta.Nodes) != 1 {
		t.Fatalf("expected only the pre-existing node to remain, got %d: %+v", len(meta.Nodes), meta.Nodes)
	}
	if _, ok := meta.Nodes[1]; !ok {
		t.Fatalf("expected node 1 preserved, got %+v", meta.Nodes)
	}
	if _, ok := meta.Nodes[2]; ok {
		t.Fatalf("expected node 2 rolled back, got %+v", meta.Nodes)
	}
}

// ---- CreateInstances: required-field validation & defaults ----

func TestCreateInstancesMissingImageFails(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	input := &backends.CreateInstanceInput{
		ClusterName: "novalid",
		Nodes:       1,
		BackendType: backends.BackendTypeVagrant,
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{},
		},
	}
	_, err := s.CreateInstances(input, 0)
	if err == nil {
		t.Fatalf("expected error for missing required Image field")
	}
	for _, c := range fr.callsSnapshot() {
		if c.method == "Up" {
			t.Fatalf("expected no Up call, got one: %+v", c)
		}
	}
}

func TestCreateInstancesParamsRecoveredByValueOrPointer(t *testing.T) {
	for _, byValue := range []bool{false, true} {
		s, fr := newVagrantTestBackend(t)
		writeValidPreflightCache(t, s)

		params := CreateInstanceParams{Image: testImage()}
		var bsp any = &params
		if byValue {
			bsp = params
		}
		input := &backends.CreateInstanceInput{
			ClusterName: "recover",
			Nodes:       1,
			BackendType: backends.BackendTypeVagrant,
			BackendSpecificParams: map[backends.BackendType]any{
				backends.BackendTypeVagrant: bsp,
			},
		}
		fr.statusResult = map[string]string{"proj-recover-1": "running"}
		if _, err := s.CreateInstances(input, 0); err != nil {
			t.Fatalf("byValue=%v: CreateInstances: %v", byValue, err)
		}
	}
}

func TestCreateInstancesDefaultsApplied(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	input := &backends.CreateInstanceInput{
		ClusterName: "defaults",
		Nodes:       1,
		BackendType: backends.BackendTypeVagrant,
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{Image: testImage()},
		},
	}
	fr.statusResult = map[string]string{"proj-defaults-1": "running"}
	if _, err := s.CreateInstances(input, 0); err != nil {
		t.Fatalf("CreateInstances: %v", err)
	}
	meta, err := s.loadClusterMeta("defaults")
	if err != nil || meta == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta)
	}
	if meta.Nodes[1].CPUs != 2 {
		t.Fatalf("expected default CPUs=2, got %d", meta.Nodes[1].CPUs)
	}
	if meta.Nodes[1].RAMMB != 2048 {
		t.Fatalf("expected default RAMMB=2048, got %d", meta.Nodes[1].RAMMB)
	}
}

// ---- CreateInstances: preflight gate ----

func TestCreateInstancesFailsFastWhenPreflightNotOK(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	// no preflight cache; make lookPath fail everything so vagrant binary "isn't found".
	s.lookPath = func(name string) (string, error) { return "", fmt.Errorf("not found: %s", name) }

	input := &backends.CreateInstanceInput{
		ClusterName: "badenv",
		Nodes:       1,
		BackendType: backends.BackendTypeVagrant,
		BackendSpecificParams: map[backends.BackendType]any{
			backends.BackendTypeVagrant: &CreateInstanceParams{Image: testImage()},
		},
	}
	_, err := s.CreateInstances(input, 0)
	if err == nil {
		t.Fatalf("expected error when preflight is not OK")
	}
	if !strings.Contains(err.Error(), "vagrant binary") {
		t.Fatalf("expected error to list preflight issues, got: %v", err)
	}
	for _, c := range fr.callsSnapshot() {
		if c.method == "Up" {
			t.Fatalf("expected no Up call, got one: %+v", c)
		}
	}
}

// ---- GetInstances ----

func TestGetInstancesMergesStatusAndMetadata(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	meta := &clusterMeta{
		ClusterName: "gi",
		ClusterUUID: "uuid-gi",
		Owner:       "jdoty",
		Tags:        map[string]string{"env": "test"},
		Nodes: map[int]*nodeMeta{
			1: {
				MachineName: "proj-gi-1",
				IP:          "192.168.56.20",
				Box:         "bento/ubuntu-22.04",
				OSName:      "ubuntu",
				OSVersion:   "22.04",
				Arch:        "amd64",
				Owner:       "jdoty",
				Expires:     time.Now().Add(time.Hour).Truncate(time.Second).UTC(),
				CreatedAt:   time.Now().Truncate(time.Second).UTC(),
				Tags:        map[string]string{"custom": "v"},
			},
			2: {
				MachineName: "proj-gi-2",
				IP:          "192.168.56.21",
			},
		},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{
		"proj-gi-1": "running",
		"proj-gi-2": "not_created",
	}

	instances, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance (not_created omitted), got %d", len(instances))
	}
	inst := instances[0]
	if inst.ZoneName != "local" || inst.ZoneID != "local" {
		t.Fatalf("expected zone local/local, got %s/%s", inst.ZoneName, inst.ZoneID)
	}
	if inst.BackendType != backends.BackendTypeVagrant {
		t.Fatalf("expected vagrant backend type, got %v", inst.BackendType)
	}
	if inst.IP.Private != "192.168.56.20" {
		t.Fatalf("expected private IP 192.168.56.20, got %s", inst.IP.Private)
	}
	if inst.InstanceID != "proj-gi-1" {
		t.Fatalf("expected InstanceID == machine name, got %s", inst.InstanceID)
	}
	if inst.Owner != "jdoty" {
		t.Fatalf("expected owner jdoty, got %s", inst.Owner)
	}
	if inst.InstanceState != backends.LifeCycleStateRunning {
		t.Fatalf("expected running state, got %v", inst.InstanceState)
	}
	if inst.OperatingSystem.Name != "ubuntu" || inst.OperatingSystem.Version != "22.04" {
		t.Fatalf("unexpected OS: %+v", inst.OperatingSystem)
	}
	if _, ok := inst.BackendSpecific.(*InstanceDetail); !ok {
		t.Fatalf("expected BackendSpecific to be *InstanceDetail, got %T", inst.BackendSpecific)
	}
}

func TestGetInstancesOmitsNotCreatedAndDefaultsMissingToStopped(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	meta := &clusterMeta{
		ClusterName: "gi2",
		ClusterUUID: "uuid-gi2",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-gi2-1", IP: "192.168.56.30"},
			2: {MachineName: "proj-gi2-2", IP: "192.168.56.31"},
		},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	// node 1 reports not_created, node 2 is absent entirely from the status map
	// (e.g. `vagrant status` doesn't know about it yet).
	fr.statusResult = map[string]string{
		"proj-gi2-1": "not_created",
	}

	instances, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}
	if len(instances) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(instances))
	}
	if instances[0].InstanceID != "proj-gi2-2" {
		t.Fatalf("expected node 2 to survive as stopped, got %s", instances[0].InstanceID)
	}
	if instances[0].InstanceState != backends.LifeCycleStateStopped {
		t.Fatalf("expected stopped state for machine missing from status map, got %v", instances[0].InstanceState)
	}
}

// TestGetInstancesReconcilesGhostNodeOnList verifies that listing prunes a ghost
// node (not_created) from an own-project cluster's on-disk metadata, keeping the
// live node, so stale entries self-heal on any inventory read.
func TestGetInstancesReconcilesGhostNodeOnList(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	if _, err := s.ensureProjectKeypair(); err != nil {
		t.Fatalf("ensureProjectKeypair: %v", err)
	}
	meta := &clusterMeta{ClusterName: "gl", ClusterUUID: "u", Nodes: map[int]*nodeMeta{
		1: {MachineName: "proj-gl-1", IP: "192.168.56.2", Box: "b"},
		2: {MachineName: "proj-gl-2", IP: "192.168.56.3", Box: "b"},
	}}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{"proj-gl-1": "running", "proj-gl-2": "not_created"}

	list, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}
	if len(list) != 1 || list[0].NodeNo != 1 {
		t.Fatalf("expected only node 1 in inventory, got %+v", list)
	}
	reloaded, err := s.loadClusterMeta("gl")
	if err != nil || reloaded == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, reloaded)
	}
	if _, ok := reloaded.Nodes[2]; ok {
		t.Fatalf("expected ghost node 2 pruned from metadata on list, got %+v", reloaded.Nodes)
	}
	if _, ok := reloaded.Nodes[1]; !ok {
		t.Fatalf("expected live node 1 retained, got %+v", reloaded.Nodes)
	}
}

// TestGetInstancesRemovesAllGhostClusterOnList verifies that listing removes the
// whole cluster dir when every node is a ghost.
func TestGetInstancesRemovesAllGhostClusterOnList(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	meta := &clusterMeta{ClusterName: "allghlist", ClusterUUID: "u", Nodes: map[int]*nodeMeta{
		1: {MachineName: "proj-allghlist-1", Box: "b"},
	}}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{"proj-allghlist-1": "not_created"}

	list, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}
	for _, i := range list {
		if i.ClusterName == "allghlist" {
			t.Fatalf("did not expect all-ghost cluster in inventory, got %+v", i)
		}
	}
	reloaded, err := s.loadClusterMeta("allghlist")
	if err != nil {
		t.Fatalf("loadClusterMeta: %v", err)
	}
	if reloaded != nil {
		t.Fatalf("expected all-ghost cluster dir removed on list, got %+v", reloaded)
	}
}

// TestGetInstancesReconcileSkipsWhenLockHeld verifies the reconciliation is
// deadlock-free: when the per-cluster lock is already held (e.g. a create/destroy
// in progress, or GetInstances called from within CreateInstances), listing skips
// reconciliation via TryLock rather than blocking, leaving metadata untouched.
func TestGetInstancesReconcileSkipsWhenLockHeld(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	meta := &clusterMeta{ClusterName: "held", ClusterUUID: "u", Nodes: map[int]*nodeMeta{
		1: {MachineName: "proj-held-1", IP: "192.168.56.2", Box: "b"},
		2: {MachineName: "proj-held-2", IP: "192.168.56.3", Box: "b"},
	}}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{"proj-held-1": "running", "proj-held-2": "not_created"}

	lock := s.lockCluster("held")
	lock.Lock()
	defer lock.Unlock()

	done := make(chan struct{})
	go func() {
		s.GetInstances(nil, nil, nil) //nolint:errcheck
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("GetInstances deadlocked while cluster lock was held")
	}

	reloaded, err := s.loadClusterMeta("held")
	if err != nil || reloaded == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, reloaded)
	}
	if _, ok := reloaded.Nodes[2]; !ok {
		t.Fatalf("expected reconciliation skipped (ghost node 2 retained) while lock held, got %+v", reloaded.Nodes)
	}
}

// ---- InstancesTerminate ----

func TestInstancesTerminate(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	meta := &clusterMeta{
		ClusterName: "term",
		ClusterUUID: "uuid-term",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-term-1", IP: "192.168.56.40"},
			2: {MachineName: "proj-term-2", IP: "192.168.56.41"},
		},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{
		"proj-term-1": "running",
		"proj-term-2": "running",
	}

	instances, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}

	// Terminate only node 1; cluster dir must survive with node 2 remaining.
	var toTerm backends.InstanceList
	for _, i := range instances {
		if i.NodeNo == 1 {
			toTerm = append(toTerm, i)
		}
	}
	if err := s.InstancesTerminate(toTerm, 0); err != nil {
		t.Fatalf("InstancesTerminate: %v", err)
	}
	found := false
	for _, c := range fr.callsSnapshot() {
		if c.method == "Destroy" && strings.Contains(c.args, "proj-term-1") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Destroy call for proj-term-1")
	}
	meta2, err := s.loadClusterMeta("term")
	if err != nil || meta2 == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta2)
	}
	if _, ok := meta2.Nodes[1]; ok {
		t.Fatalf("expected node 1 removed from metadata")
	}
	if _, ok := meta2.Nodes[2]; !ok {
		t.Fatalf("expected node 2 to remain in metadata")
	}

	// Terminate the last remaining node; whole cluster dir must be removed.
	var toTerm2 backends.InstanceList
	for _, i := range instances {
		if i.NodeNo == 2 {
			toTerm2 = append(toTerm2, i)
		}
	}
	if err := s.InstancesTerminate(toTerm2, 0); err != nil {
		t.Fatalf("InstancesTerminate (last node): %v", err)
	}
	meta3, err := s.loadClusterMeta("term")
	if err != nil {
		t.Fatalf("loadClusterMeta: %v", err)
	}
	if meta3 != nil {
		t.Fatalf("expected cluster dir removed after last node terminated, got %+v", meta3)
	}
	if _, err := os.Stat(s.clusterDir("term")); !os.IsNotExist(err) {
		t.Fatalf("expected cluster directory to no longer exist")
	}
}

// ---- InstancesStop / InstancesStart ----

func TestInstancesStopHaltsAndTerminateOnStopDestroys(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	meta := &clusterMeta{
		ClusterName: "stop",
		ClusterUUID: "uuid-stop",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-stop-1", IP: "192.168.56.50"},
			2: {MachineName: "proj-stop-2", IP: "192.168.56.51", TerminateOnStop: true},
		},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{
		"proj-stop-1": "running",
		"proj-stop-2": "running",
	}
	instances, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}

	if err := s.InstancesStop(instances, false, 0); err != nil {
		t.Fatalf("InstancesStop: %v", err)
	}

	var haltArgs, destroyArgs string
	for _, c := range fr.callsSnapshot() {
		switch c.method {
		case "Halt":
			haltArgs = c.args
		case "Destroy":
			destroyArgs = c.args
		}
	}
	if !strings.Contains(haltArgs, "proj-stop-1") || strings.Contains(haltArgs, "proj-stop-2") {
		t.Fatalf("expected Halt for stop-1 only, got: %s", haltArgs)
	}
	if !strings.Contains(destroyArgs, "proj-stop-2") {
		t.Fatalf("expected Destroy for TerminateOnStop node stop-2, got: %s", destroyArgs)
	}

	meta2, err := s.loadClusterMeta("stop")
	if err != nil || meta2 == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta2)
	}
	if _, ok := meta2.Nodes[2]; ok {
		t.Fatalf("expected TerminateOnStop node removed from metadata")
	}
	if _, ok := meta2.Nodes[1]; !ok {
		t.Fatalf("expected regular stopped node to remain in metadata")
	}
}

func TestInstancesStartUsesExistingMachines(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	meta := &clusterMeta{
		ClusterName: "start",
		ClusterUUID: "uuid-start",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-start-1", IP: "192.168.56.60"},
		},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{"proj-start-1": "poweroff"}
	instances, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}

	if err := s.InstancesStart(instances, 0); err != nil {
		t.Fatalf("InstancesStart: %v", err)
	}
	found := false
	for _, c := range fr.callsSnapshot() {
		if c.method == "Up" && strings.Contains(c.args, "proj-start-1") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Up call with existing machine name proj-start-1")
	}
}

// The SSH-ready poll after InstancesStart must see refreshed (running) instance
// state, not the caller's pre-start stopped snapshots — the exec path refuses
// non-running instances, so polling stale snapshots can never succeed.
func TestInstancesStartPollsRefreshedState(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	meta := &clusterMeta{
		ClusterName: "startpoll",
		ClusterUUID: "uuid-startpoll",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-startpoll-1", IP: "192.168.56.61"},
		},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{"proj-startpoll-1": "poweroff"}
	instances, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}
	if instances[0].InstanceState != backends.LifeCycleStateStopped {
		t.Fatalf("precondition: expected stopped snapshot, got %v", instances[0].InstanceState)
	}

	// after Up, status reports running; the poll must observe that
	fr.statusResult = map[string]string{"proj-startpoll-1": "running"}
	var polled backends.InstanceList
	s.sshReadyPoll = func(insts backends.InstanceList, _ time.Duration) error {
		polled = insts
		return nil
	}
	if err := s.InstancesStart(instances, time.Minute); err != nil {
		t.Fatalf("InstancesStart: %v", err)
	}
	if len(polled) != 1 {
		t.Fatalf("expected 1 polled instance, got %d", len(polled))
	}
	if polled[0].InstanceState != backends.LifeCycleStateRunning {
		t.Fatalf("poll must receive refreshed running state, got %v", polled[0].InstanceState)
	}
}

// ---- InstancesAddTags / InstancesRemoveTags ----

func TestInstancesAddRemoveTags(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	meta := &clusterMeta{
		ClusterName: "tags",
		ClusterUUID: "uuid-tags",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-tags-1", IP: "192.168.56.70", Tags: map[string]string{"a": "1"}},
		},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{"proj-tags-1": "running"}
	instances, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}

	if err := s.InstancesAddTags(instances, map[string]string{"b": "2"}); err != nil {
		t.Fatalf("InstancesAddTags: %v", err)
	}
	for _, c := range fr.callsSnapshot() {
		if c.method != "Status" {
			t.Fatalf("expected no runner calls beyond Status, got: %+v", c)
		}
	}
	instances2, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances (after add): %v", err)
	}
	if instances2[0].Tags["a"] != "1" || instances2[0].Tags["b"] != "2" {
		t.Fatalf("expected merged tags, got %+v", instances2[0].Tags)
	}

	if err := s.InstancesRemoveTags(instances2, []string{"a"}); err != nil {
		t.Fatalf("InstancesRemoveTags: %v", err)
	}
	instances3, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances (after remove): %v", err)
	}
	if _, ok := instances3[0].Tags["a"]; ok {
		t.Fatalf("expected tag 'a' removed, got %+v", instances3[0].Tags)
	}
	if instances3[0].Tags["b"] != "2" {
		t.Fatalf("expected tag 'b' to remain, got %+v", instances3[0].Tags)
	}
}

// ---- InstancesChangeExpiry ----

func TestInstancesChangeExpiry(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	meta := &clusterMeta{
		ClusterName: "expiry",
		ClusterUUID: "uuid-expiry",
		Nodes: map[int]*nodeMeta{
			1: {MachineName: "proj-expiry-1", IP: "192.168.56.80"},
		},
	}
	if err := s.saveClusterMeta(meta); err != nil {
		t.Fatalf("saveClusterMeta: %v", err)
	}
	fr.statusResult = map[string]string{"proj-expiry-1": "running"}
	instances, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances: %v", err)
	}

	newExpiry := time.Now().Add(48 * time.Hour).Truncate(time.Second).UTC()
	if err := s.InstancesChangeExpiry(instances, newExpiry); err != nil {
		t.Fatalf("InstancesChangeExpiry: %v", err)
	}

	instances2, err := s.GetInstances(nil, nil, nil)
	if err != nil {
		t.Fatalf("GetInstances (after expiry change): %v", err)
	}
	if !instances2[0].Expires.Equal(newExpiry) {
		t.Fatalf("expected Expires=%v, got %v", newExpiry, instances2[0].Expires)
	}
	if instances2[0].Tags[TAG_EXPIRES] != newExpiry.Format(time.RFC3339) {
		t.Fatalf("expected TAG_EXPIRES tag set, got %+v", instances2[0].Tags)
	}
}

// ---- Per-cluster locking ----

// TestConcurrentCreateInstancesSerializePerCluster fires two concurrent
// CreateInstances calls at the same cluster. If they aren't serialized by
// lockCluster, both would read the same "no existing nodes" metadata state and
// both would allocate node 1, corrupting the metadata (and racing under -race).
// fakeRunner.onUp records concurrent overlap directly as a second signal.
func TestConcurrentCreateInstancesSerializePerCluster(t *testing.T) {
	s, fr := newVagrantTestBackend(t)
	writeValidPreflightCache(t, s)

	fr.statusResult = map[string]string{
		"proj-lockcluster-1": "running",
		"proj-lockcluster-2": "running",
	}

	var mu sync.Mutex
	active := 0
	maxActive := 0
	fr.onUp = func(dir string, machines []string, provider string) error {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		time.Sleep(20 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		return nil
	}

	newInput := func() *backends.CreateInstanceInput {
		return &backends.CreateInstanceInput{
			ClusterName: "lockcluster",
			Nodes:       1,
			BackendType: backends.BackendTypeVagrant,
			BackendSpecificParams: map[backends.BackendType]any{
				backends.BackendTypeVagrant: &CreateInstanceParams{Image: testImage()},
			},
		}
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if _, err := s.CreateInstances(newInput(), 0); err != nil {
			t.Errorf("CreateInstances (goroutine 1): %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		if _, err := s.CreateInstances(newInput(), 0); err != nil {
			t.Errorf("CreateInstances (goroutine 2): %v", err)
		}
	}()
	wg.Wait()

	if maxActive > 1 {
		t.Fatalf("expected serialized access to cluster, saw %d concurrent Up calls", maxActive)
	}

	meta, err := s.loadClusterMeta("lockcluster")
	if err != nil || meta == nil {
		t.Fatalf("loadClusterMeta: %v %+v", err, meta)
	}
	if len(meta.Nodes) != 2 {
		t.Fatalf("expected 2 nodes total (numbering serialized, no collision), got %d: %+v", len(meta.Nodes), meta.Nodes)
	}
	if _, ok := meta.Nodes[1]; !ok {
		t.Fatalf("expected node 1 to exist")
	}
	if _, ok := meta.Nodes[2]; !ok {
		t.Fatalf("expected node 2 to exist")
	}
}
