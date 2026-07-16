package bvagrant

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/structtags"
	"github.com/google/uuid"
	"golang.org/x/crypto/ssh"
)

// CreateInstanceParams is the vagrant-backend-specific payload expected in
// backends.CreateInstanceInput.BackendSpecificParams[backends.BackendTypeVagrant].
type CreateInstanceParams struct {
	// Image describes the OS/version/arch to resolve to a vagrant box via boxNaming,
	// unless Box is set explicitly.
	Image *backends.Image `yaml:"image" json:"image" required:"true"`
	// Box, if set, overrides image-based box resolution.
	Box string `yaml:"box" json:"box"`
	// Provider is passed as --provider to vagrant up; empty falls back to
	// credentials.DefaultProvider, then to vagrant's own default resolution.
	Provider string `yaml:"provider" json:"provider"`
	CPUs     int    `yaml:"cpus" json:"cpus"`
	RAMMB    int    `yaml:"ramMB" json:"ramMB"`
	// Disks entries are "volumeName:/guest/path[:ro]", same format parseSyncedFolders expects.
	Disks []string `yaml:"disks" json:"disks"`
}

// InstanceDetail is the vagrant-specific payload attached to backends.Instance.BackendSpecific.
type InstanceDetail struct {
	ClusterDir  string
	MachineName string
	Provider    string
	RawState    string
}

// mapVagrantState maps a raw `vagrant status` machine state string to a LifeCycleState.
// "not_created" is handled by the caller (it means the machine has no metadata-backed
// existence yet and is omitted from inventory, not mapped to a state here).
func mapVagrantState(raw string) backends.LifeCycleState {
	switch raw {
	case "running":
		return backends.LifeCycleStateRunning
	case "poweroff", "saved", "aborted":
		return backends.LifeCycleStateStopped
	default:
		return backends.LifeCycleStateUnknown
	}
}

// ensureProjectKeypair returns the project's SSH public key (authorized_keys line),
// generating a fresh RSA-2048 keypair at sshKeysDir/{project,project.pub} the first
// time it's needed and reusing it thereafter. Mirrors bdocker's CreateInstances keypair
// logic (crypto/rsa, x509 PEM private key, ssh.MarshalAuthorizedKey public key).
func (s *b) ensureProjectKeypair() (string, error) {
	keyPath := filepath.Join(s.sshKeysDir, s.project)
	pubPath := keyPath + ".pub"

	if _, err := os.Stat(keyPath); os.IsNotExist(err) {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return "", fmt.Errorf("failed to generate private key: %w", err)
		}
		pub, err := ssh.NewPublicKey(&privateKey.PublicKey)
		if err != nil {
			return "", fmt.Errorf("failed to create public key: %w", err)
		}
		pubBytes := ssh.MarshalAuthorizedKey(pub)
		privBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		})
		if err := os.MkdirAll(s.sshKeysDir, 0700); err != nil {
			return "", fmt.Errorf("failed to create ssh keys directory: %w", err)
		}
		if err := os.WriteFile(keyPath, privBytes, 0600); err != nil {
			return "", fmt.Errorf("failed to save private key: %w", err)
		}
		if err := os.WriteFile(pubPath, pubBytes, 0600); err != nil {
			return "", fmt.Errorf("failed to save public key: %w", err)
		}
		return strings.TrimSpace(string(pubBytes)), nil
	} else if err != nil {
		return "", err
	}

	pubBytes, err := os.ReadFile(pubPath)
	if err != nil {
		return "", fmt.Errorf("failed to read public key: %w", err)
	}
	return strings.TrimSpace(string(pubBytes)), nil
}

// readProjectPubKey returns the project's existing SSH public key without
// generating one (unlike ensureProjectKeypair). It is used on read paths such as
// ghost reconciliation during listing, where creating a keypair as a side effect
// of a list would be surprising; callers treat an error as "skip Vagrantfile
// regeneration". A cluster with on-disk metadata always has a keypair already.
func (s *b) readProjectPubKey() (string, error) {
	pubBytes, err := os.ReadFile(filepath.Join(s.sshKeysDir, s.project) + ".pub")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(pubBytes)), nil
}

// recoverCreateInstanceParams extracts and validates the vagrant-specific params from
// input.BackendSpecificParams, accepting either a *CreateInstanceParams or a
// CreateInstanceParams value (mirrors bdocker's recovery pattern).
func recoverCreateInstanceParams(input *backends.CreateInstanceInput) (*CreateInstanceParams, error) {
	params := &CreateInstanceParams{}
	if input.BackendSpecificParams != nil {
		if raw, ok := input.BackendSpecificParams[backends.BackendTypeVagrant]; ok {
			switch v := raw.(type) {
			case *CreateInstanceParams:
				params = v
			case CreateInstanceParams:
				params = &v
			default:
				return nil, fmt.Errorf("invalid backend-specific parameters for vagrant")
			}
		}
	}
	if err := structtags.CheckRequired(params); err != nil {
		return nil, fmt.Errorf("required fields missing in backend-specific parameters: %w", err)
	}
	return params, nil
}

// CreateInstances creates input.Nodes new nodes in input.ClusterName, growing existing
// metadata if the cluster already exists. Flow: recover/validate params, Preflight
// (fail fast if not OK), lock the cluster, resolve the box, ensure the project keypair,
// load-or-create metadata, allocate node numbers/IPs, write metadata + Vagrantfile,
// `vagrant up` only the newly allocated machines, then refresh via GetInstances and
// (optionally) wait for SSH readiness.
func (s *b) CreateInstances(input *backends.CreateInstanceInput, waitDur time.Duration) (*backends.CreateInstanceOutput, error) {
	params, err := recoverCreateInstanceParams(input)
	if err != nil {
		return nil, err
	}

	pf, err := s.Preflight(false)
	if err != nil {
		return nil, err
	}
	if !pf.OK {
		return nil, fmt.Errorf("vagrant preflight failed: %s", strings.Join(pf.Issues, "; "))
	}

	lock := s.lockCluster(input.ClusterName)
	lock.Lock()
	defer lock.Unlock()

	// Box resolution order: explicit override, then the image's own box name
	// (ImageId is the vagrant box for both catalog templates and custom images
	// — using it directly is what makes custom/template images actually boot
	// from their packaged box), then the distro catalog as a fallback for
	// hand-constructed Image values without an ImageId.
	box := params.Box
	if box == "" {
		box = params.Image.ImageId
	}
	if box == "" {
		box, err = boxNaming(params.Image.OSName, params.Image.OSVersion, params.Image.Architecture.String())
		if err != nil {
			return nil, err
		}
	}

	pubKey, err := s.ensureProjectKeypair()
	if err != nil {
		return nil, err
	}

	meta, err := s.loadClusterMeta(input.ClusterName)
	if err != nil {
		return nil, err
	}
	if meta == nil {
		meta = &clusterMeta{
			ClusterName: input.ClusterName,
			ClusterUUID: uuid.New().String(),
			Nodes:       map[int]*nodeMeta{},
		}
	}
	if meta.Nodes == nil {
		meta.Nodes = map[int]*nodeMeta{}
	}
	if meta.Owner == "" {
		meta.Owner = input.Owner
	}
	if meta.Tags == nil {
		meta.Tags = map[string]string{}
	}
	maps.Copy(meta.Tags, input.Tags)

	// Reconcile ghost nodes before allocating: a node whose VM no longer exists
	// (reported "not_created" by `vagrant status` — e.g. destroyed outside
	// aerolab or left behind by a prior failed create) lingers in metadata but is
	// filtered out of the live inventory (see instancesForCluster). Left in place
	// it silently inflates node numbering (the next node is lastNodeNo+1, counting
	// ghosts) and can never be cleaned via `cluster destroy`, which operates on the
	// inventory. Prune ghosts here so numbering matches what the user sees and
	// orphaned metadata self-heals. Skipped for a brand-new cluster (no nodes yet,
	// and no Vagrantfile to query status against).
	if len(meta.Nodes) > 0 {
		if err := s.pruneGhostNodes(meta, pubKey); err != nil {
			return nil, err
		}
	}

	lastNodeNo := 0
	for no := range meta.Nodes {
		if no > lastNodeNo {
			lastNodeNo = no
		}
	}

	ips, err := s.allocateIPs(input.Nodes)
	if err != nil {
		return nil, err
	}

	provider := params.Provider
	if provider == "" && s.credentials != nil {
		provider = s.credentials.DefaultProvider
	}

	cpus := params.CPUs
	if cpus <= 0 {
		cpus = defaultVagrantCPUs
	}
	ram := params.RAMMB
	if ram <= 0 {
		ram = defaultVagrantRAMMB
	}

	baseTags := map[string]string{
		TAG_OWNER:           input.Owner,
		TAG_CLUSTER_NAME:    input.ClusterName,
		TAG_DESCRIPTION:     input.Description,
		TAG_AEROLAB_PROJECT: s.project,
		TAG_AEROLAB_VERSION: s.aerolabVersion,
		TAG_OS_NAME:         params.Image.OSName,
		TAG_OS_VERSION:      params.Image.OSVersion,
		TAG_ARCHITECTURE:    params.Image.Architecture.String(),
		TAG_CLUSTER_UUID:    meta.ClusterUUID,
	}
	if !input.Expires.IsZero() {
		baseTags[TAG_EXPIRES] = input.Expires.Format(time.RFC3339)
	}
	maps.Copy(baseTags, input.Tags)

	newMachines := make([]string, 0, input.Nodes)
	newNodeNos := make([]int, 0, input.Nodes)
	for i := 1; i <= input.Nodes; i++ {
		nodeNo := lastNodeNo + i
		machineName := s.machineName(s.project, input.ClusterName, nodeNo)

		nodeTags := make(map[string]string, len(baseTags)+2)
		maps.Copy(nodeTags, baseTags)
		nodeTags[TAG_NODE_NO] = fmt.Sprintf("%d", nodeNo)
		name := input.Name
		if name == "" {
			name = machineName
		}
		nodeTags[TAG_NAME] = name

		meta.Nodes[nodeNo] = &nodeMeta{
			MachineName:     machineName,
			IP:              ips[i-1],
			Box:             box,
			OSName:          params.Image.OSName,
			OSVersion:       params.Image.OSVersion,
			Arch:            params.Image.Architecture.String(),
			CPUs:            cpus,
			RAMMB:           ram,
			Disks:           params.Disks,
			Tags:            nodeTags,
			Expires:         input.Expires,
			Description:     input.Description,
			Owner:           input.Owner,
			CreatedAt:       time.Now(),
			TerminateOnStop: input.TerminateOnStop,
			Provider:        provider,
		}
		newMachines = append(newMachines, machineName)
		newNodeNos = append(newNodeNos, nodeNo)
	}

	if err := s.saveClusterMeta(meta); err != nil {
		return nil, err
	}
	if err := s.writeVagrantfile(meta, pubKey); err != nil {
		return nil, err
	}

	if err := s.runner.Up(s.clusterDir(input.ClusterName), newMachines, provider, false); err != nil {
		s.rollbackFailedCreate(input.ClusterName, meta, newMachines, newNodeNos, pubKey)
		return nil, fmt.Errorf("vagrant up failed: %w", err)
	}

	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateInstance) //nolint:errcheck
	}

	all, err := s.GetInstances(s.volumes, s.networks, s.firewalls)
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %w", err)
	}

	newSet := make(map[string]bool, len(newMachines))
	for _, m := range newMachines {
		newSet[m] = true
	}
	output := &backends.CreateInstanceOutput{Instances: backends.InstanceList{}}
	for _, inst := range all {
		if inst.ClusterName == input.ClusterName && newSet[inst.InstanceID] {
			output.Instances = append(output.Instances, inst)
		}
	}
	sort.Slice(output.Instances, func(i, j int) bool { return output.Instances[i].NodeNo < output.Instances[j].NodeNo })

	if waitDur > 0 {
		poll := s.sshReadyPoll
		if poll == nil {
			poll = s.defaultSSHReadyPoll
		}
		if err := poll(output.Instances, waitDur); err != nil {
			return nil, err
		}
	}

	return output, nil
}

// pruneGhostNodes removes nodes whose VM no longer exists (reported "not_created"
// by `vagrant status`) from meta, persisting the pruned metadata and regenerating
// the Vagrantfile when anything changed. Absent-from-status machines are kept, to
// stay consistent with instancesForCluster (which treats them as stopped, not gone).
// A status-query failure is non-fatal: reconciliation is a best-effort self-heal, so
// on error we log and leave metadata untouched rather than block the create.
func (s *b) pruneGhostNodes(meta *clusterMeta, pubKey string) error {
	statusMap, err := s.runner.Status(s.clusterDir(meta.ClusterName))
	if err != nil {
		if s.log != nil {
			s.log.Warn("VAGRANT: skipping ghost-node reconciliation for cluster %q: %v", meta.ClusterName, err)
		}
		return nil
	}
	if !removeGhostNodes(meta, statusMap) {
		return nil
	}
	if len(meta.Nodes) == 0 {
		return s.deleteClusterMeta(meta.ClusterName)
	}
	if err := s.saveClusterMeta(meta); err != nil {
		return err
	}
	return s.writeVagrantfile(meta, pubKey)
}

// rollbackFailedCreate undoes a create whose `vagrant up` failed: it destroys the
// machines that were being brought up (a boot timeout can leave a VM running but
// unprovisioned) and drops their nodes from metadata, so a failed create leaves no
// ghost VM behind and does not inflate future node numbering. Best-effort: errors
// are logged rather than returned, since the create is already failing and the
// original `vagrant up` error is what the caller should see.
func (s *b) rollbackFailedCreate(clusterName string, meta *clusterMeta, machines []string, nodeNos []int, pubKey string) {
	if derr := s.runner.Destroy(s.clusterDir(clusterName), machines); derr != nil && s.log != nil {
		s.log.Warn("VAGRANT: rollback destroy after failed up for cluster %q returned: %v", clusterName, derr)
	}
	for _, no := range nodeNos {
		delete(meta.Nodes, no)
	}
	var err error
	if len(meta.Nodes) == 0 {
		err = s.deleteClusterMeta(clusterName)
	} else if err = s.saveClusterMeta(meta); err == nil {
		err = s.writeVagrantfile(meta, pubKey)
	}
	if err != nil && s.log != nil {
		s.log.Warn("VAGRANT: rollback metadata cleanup after failed up for cluster %q returned: %v", clusterName, err)
	}
}

// defaultSSHReadyPoll is the production sshReadyPoll implementation: it polls each
// instance with a lightweight `ls /` exec (via InstancesExec, wired to real SSH in
// Phase 4) until all respond or the budget runs out.
func (s *b) defaultSSHReadyPoll(instances backends.InstanceList, waitDur time.Duration) error {
	if len(instances) == 0 {
		return nil
	}
	deadline := time.Now().Add(waitDur)
	for {
		out := s.InstancesExec(instances, &backends.ExecInput{
			Username:        "root",
			ParallelThreads: len(instances),
			ConnectTimeout:  5 * time.Second,
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"ls", "/"},
				SessionTimeout: 10 * time.Second,
			},
		})
		success := len(out) == len(instances)
		for _, o := range out {
			if o.Output != nil && o.Output.Err != nil {
				success = false
			}
		}
		if success {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("instances failed to become ssh-ready within %s", waitDur)
		}
		time.Sleep(time.Second)
	}
}

// GetInstances lists instances from every cluster's on-disk metadata, merged with live
// `vagrant status` per cluster. When listAllProjects is set, sibling project directories
// under RootDir (configDir's grandparent) are scanned too — configDir is laid out as
// RootDir/<project>/config/vagrant (see backends.Backend.SetConfig callers), so sibling
// projects' vagrant cluster metadata lives at RootDir/<otherProject>/config/vagrant/clusters.
func (s *b) GetInstances(volumes backends.VolumeList, networks backends.NetworkList, firewalls backends.FirewallList) (backends.InstanceList, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("vagrant runner is not configured")
	}

	metas, err := s.listClusterMetas()
	if err != nil {
		return nil, err
	}

	var out backends.InstanceList
	for _, meta := range metas {
		dir := s.clusterDir(meta.ClusterName)
		statusMap, err := s.runner.Status(dir)
		if err != nil {
			return nil, err
		}
		// Reconcile ghost nodes for own-project clusters: prune nodes whose VM
		// no longer exists (not_created) so stale metadata doesn't linger in the
		// inventory or keep an orphaned cluster dir alive. Returns nil when the
		// whole cluster was ghost and its dir was removed. Not applied to
		// other-project clusters below — we never mutate another project's state.
		meta = s.reconcileClusterGhosts(meta, statusMap)
		if meta == nil {
			continue
		}
		out = append(out, s.buildInstances(meta, statusMap, dir)...)
	}

	if s.listAllProjects {
		others, err := s.otherProjectClusterMetas()
		if err != nil {
			return nil, err
		}
		for _, oc := range others {
			insts, err := s.instancesForCluster(oc.meta, oc.dir)
			if err != nil {
				return nil, err
			}
			out = append(out, insts...)
		}
	}

	s.instances = out
	return out, nil
}

// instancesForCluster fetches `vagrant status` for a cluster dir and builds its
// instance list (see buildInstances). It performs no ghost reconciliation, so it
// is safe for read-only use over other-project clusters, whose metadata we must
// not mutate.
func (s *b) instancesForCluster(meta *clusterMeta, dir string) (backends.InstanceList, error) {
	statusMap, err := s.runner.Status(dir)
	if err != nil {
		return nil, err
	}
	return s.buildInstances(meta, statusMap, dir), nil
}

// hasGhost reports whether meta contains any node whose VM no longer exists
// (status not_created), or a nil node entry.
func hasGhost(meta *clusterMeta, statusMap map[string]string) bool {
	for _, node := range meta.Nodes {
		if node == nil || statusMap[node.MachineName] == "not_created" {
			return true
		}
	}
	return false
}

// removeGhostNodes deletes nodes whose VM no longer exists (status not_created),
// and nil entries, from meta in-memory. Returns true if anything was removed.
// It does not persist; callers decide how to save.
func removeGhostNodes(meta *clusterMeta, statusMap map[string]string) bool {
	removed := false
	for no, node := range meta.Nodes {
		if node == nil || statusMap[node.MachineName] == "not_created" {
			delete(meta.Nodes, no)
			removed = true
		}
	}
	return removed
}

// reconcileClusterGhosts prunes ghost nodes from an own-project cluster's
// metadata during inventory listing, persisting the change (or removing the
// cluster dir entirely when every node was a ghost). It returns the metadata to
// build instances from, or nil when the cluster was fully removed. The status
// map is computed by the caller; a fast hasGhost check avoids taking any lock
// when there is nothing to reconcile. TryLock (rather than Lock) keeps this
// best-effort and deadlock-free: GetInstances is called from within
// CreateInstances while the per-cluster lock is already held, and a
// create/destroy may hold it concurrently — in either case we simply skip
// reconciliation this pass and leave metadata untouched.
func (s *b) reconcileClusterGhosts(meta *clusterMeta, statusMap map[string]string) *clusterMeta {
	if !hasGhost(meta, statusMap) {
		return meta
	}
	lock := s.lockCluster(meta.ClusterName)
	if !lock.TryLock() {
		return meta
	}
	defer lock.Unlock()

	// Re-load under the lock so we act on the authoritative on-disk state, not a
	// snapshot that a concurrent create/destroy may have superseded.
	fresh, err := s.loadClusterMeta(meta.ClusterName)
	if err != nil {
		if s.log != nil {
			s.log.Warn("VAGRANT: ghost reconciliation reload for cluster %q failed: %v", meta.ClusterName, err)
		}
		return meta
	}
	if fresh == nil {
		return nil
	}
	if !removeGhostNodes(fresh, statusMap) {
		return fresh
	}
	if len(fresh.Nodes) == 0 {
		if err := s.deleteClusterMeta(fresh.ClusterName); err != nil && s.log != nil {
			s.log.Warn("VAGRANT: removing all-ghost cluster %q failed: %v", fresh.ClusterName, err)
		}
		return nil
	}
	if err := s.saveClusterMeta(fresh); err != nil {
		if s.log != nil {
			s.log.Warn("VAGRANT: persisting ghost reconciliation for cluster %q failed: %v", fresh.ClusterName, err)
		}
		return fresh
	}
	if pub, err := s.readProjectPubKey(); err == nil {
		if werr := s.writeVagrantfile(fresh, pub); werr != nil && s.log != nil {
			s.log.Warn("VAGRANT: regenerating Vagrantfile for cluster %q after ghost reconciliation failed: %v", fresh.ClusterName, werr)
		}
	}
	return fresh
}

// buildInstances merges a cluster's metadata with a `vagrant status` map (from
// dir) into backends.Instance values. A machine reported as "not_created" is
// omitted (it exists in metadata but vagrant has no real state for it). A
// machine present in metadata but absent from the status map (e.g. `vagrant
// status` hasn't seen it commit yet) defaults to LifeCycleStateStopped, since
// that's the safer assumption for a node that isn't confirmed running.
func (s *b) buildInstances(meta *clusterMeta, statusMap map[string]string, dir string) backends.InstanceList {
	nodeNos := make([]int, 0, len(meta.Nodes))
	for no := range meta.Nodes {
		nodeNos = append(nodeNos, no)
	}
	sort.Ints(nodeNos)

	var out backends.InstanceList
	for _, no := range nodeNos {
		node := meta.Nodes[no]
		if node == nil {
			continue
		}
		raw, present := statusMap[node.MachineName]
		if present && raw == "not_created" {
			continue
		}
		state := backends.LifeCycleStateStopped
		if present {
			state = mapVagrantState(raw)
		}

		var arch backends.Architecture
		arch.FromString(node.Arch) //nolint:errcheck

		out = append(out, &backends.Instance{
			ClusterName:  meta.ClusterName,
			ClusterUUID:  meta.ClusterUUID,
			NodeNo:       no,
			IP:           backends.IP{Private: node.IP},
			Architecture: arch,
			OperatingSystem: backends.OS{
				Name:    node.OSName,
				Version: node.OSVersion,
			},
			InstanceID:    node.MachineName,
			BackendType:   backends.BackendTypeVagrant,
			Name:          node.MachineName,
			ZoneName:      "local",
			ZoneID:        "local",
			CreationTime:  node.CreatedAt,
			Owner:         node.Owner,
			InstanceState: state,
			Tags:          node.Tags,
			Expires:       node.Expires,
			Description:   node.Description,
			BackendSpecific: &InstanceDetail{
				ClusterDir:  dir,
				MachineName: node.MachineName,
				Provider:    node.Provider,
				RawState:    raw,
			},
		})
	}
	return out
}

type otherProjectCluster struct {
	meta *clusterMeta
	dir  string
}

// otherProjectClusterMetas scans RootDir/<otherProject>/config/vagrant/clusters for
// every project sibling to s.project. A project directory that can't be scanned (e.g.
// no vagrant config for that project) is skipped, matching listClusterMetas' best-effort
// behavior for a single project's scan.
func (s *b) otherProjectClusterMetas() ([]otherProjectCluster, error) {
	rootDir := filepath.Dir(filepath.Dir(filepath.Dir(s.configDir)))
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var result []otherProjectCluster
	for _, e := range entries {
		if !e.IsDir() || e.Name() == s.project {
			continue
		}
		otherClustersRoot := filepath.Join(rootDir, e.Name(), "config", "vagrant", "clusters")
		metas, err := s.listClusterMetasAt(otherClustersRoot)
		if err != nil {
			continue
		}
		for _, m := range metas {
			result = append(result, otherProjectCluster{meta: m, dir: filepath.Join(otherClustersRoot, m.ClusterName)})
		}
	}
	return result, nil
}

// groupInstancesByCluster splits an InstanceList into per-cluster-name buckets, the
// common first step for every lifecycle operation below (each cluster is locked and
// its metadata mutated independently).
func groupInstancesByCluster(instances backends.InstanceList) map[string]backends.InstanceList {
	byCluster := make(map[string]backends.InstanceList)
	for _, inst := range instances {
		byCluster[inst.ClusterName] = append(byCluster[inst.ClusterName], inst)
	}
	return byCluster
}

// InstancesTerminate destroys the given instances (vagrant destroy handles running VMs
// directly, no separate stop-then-destroy step needed) and removes them from cluster
// metadata. If a cluster's last node is removed, the whole cluster directory (metadata +
// Vagrantfile) is removed; otherwise the Vagrantfile is regenerated for the remaining nodes.
func (s *b) InstancesTerminate(instances backends.InstanceList, waitDur time.Duration) error {
	if len(instances) == 0 {
		return nil
	}
	for clusterName, insts := range groupInstancesByCluster(instances) {
		if err := s.withClusterLock(clusterName, func() error {
			meta, err := s.loadClusterMeta(clusterName)
			if err != nil {
				return err
			}
			if meta == nil {
				return nil
			}
			machines := make([]string, 0, len(insts))
			for _, inst := range insts {
				machines = append(machines, inst.InstanceID)
			}
			if err := s.runner.Destroy(s.clusterDir(clusterName), machines); err != nil {
				return err
			}
			for _, inst := range insts {
				delete(meta.Nodes, inst.NodeNo)
			}
			if len(meta.Nodes) == 0 {
				return s.deleteClusterMeta(clusterName)
			}
			if err := s.saveClusterMeta(meta); err != nil {
				return err
			}
			pubKey, err := s.ensureProjectKeypair()
			if err != nil {
				return err
			}
			return s.writeVagrantfile(meta, pubKey)
		}); err != nil {
			return err
		}
	}
	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateInstance) //nolint:errcheck
	}
	return nil
}

// InstancesStop halts the given instances. Nodes created with TerminateOnStop are
// destroyed instead (and removed from metadata) rather than halted, matching the
// documented "terminate on stop" semantics.
func (s *b) InstancesStop(instances backends.InstanceList, force bool, waitDur time.Duration) error {
	if len(instances) == 0 {
		return nil
	}
	for clusterName, insts := range groupInstancesByCluster(instances) {
		if err := s.withClusterLock(clusterName, func() error {
			meta, err := s.loadClusterMeta(clusterName)
			if err != nil {
				return err
			}
			if meta == nil {
				return nil
			}

			var haltMachines, destroyMachines []string
			var destroyNodeNos []int
			for _, inst := range insts {
				node := meta.Nodes[inst.NodeNo]
				if node != nil && node.TerminateOnStop {
					destroyMachines = append(destroyMachines, inst.InstanceID)
					destroyNodeNos = append(destroyNodeNos, inst.NodeNo)
				} else {
					haltMachines = append(haltMachines, inst.InstanceID)
				}
			}

			if len(haltMachines) > 0 {
				if err := s.runner.Halt(s.clusterDir(clusterName), haltMachines); err != nil {
					return err
				}
			}
			if len(destroyMachines) == 0 {
				return nil
			}
			if err := s.runner.Destroy(s.clusterDir(clusterName), destroyMachines); err != nil {
				return err
			}
			for _, no := range destroyNodeNos {
				delete(meta.Nodes, no)
			}
			if len(meta.Nodes) == 0 {
				return s.deleteClusterMeta(clusterName)
			}
			if err := s.saveClusterMeta(meta); err != nil {
				return err
			}
			pubKey, err := s.ensureProjectKeypair()
			if err != nil {
				return err
			}
			return s.writeVagrantfile(meta, pubKey)
		}); err != nil {
			return err
		}
	}
	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateInstance) //nolint:errcheck
	}
	return nil
}

// InstancesStart brings up existing machines (no new node allocation, unlike
// CreateInstances) and optionally waits for them to become ssh-ready.
func (s *b) InstancesStart(instances backends.InstanceList, waitDur time.Duration) error {
	if len(instances) == 0 {
		return nil
	}
	for clusterName, insts := range groupInstancesByCluster(instances) {
		if err := s.withClusterLock(clusterName, func() error {
			meta, err := s.loadClusterMeta(clusterName)
			if err != nil {
				return err
			}
			if meta == nil {
				return fmt.Errorf("cluster %q not found", clusterName)
			}

			machines := make([]string, 0, len(insts))
			provider := ""
			for _, inst := range insts {
				machines = append(machines, inst.InstanceID)
				if provider == "" {
					if node := meta.Nodes[inst.NodeNo]; node != nil && node.Provider != "" {
						provider = node.Provider
					}
				}
			}
			if provider == "" && s.credentials != nil {
				provider = s.credentials.DefaultProvider
			}
			return s.runner.Up(s.clusterDir(clusterName), machines, provider, false)
		}); err != nil {
			return err
		}
	}
	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateInstance) //nolint:errcheck
	}
	if waitDur > 0 {
		// re-read state before polling: the caller's instances carry the
		// pre-start (stopped) snapshot, and the exec path refuses instances
		// that aren't running, so polling the stale snapshots can never succeed
		fresh, err := s.refreshInstances(instances)
		if err != nil {
			return err
		}
		poll := s.sshReadyPoll
		if poll == nil {
			poll = s.defaultSSHReadyPoll
		}
		return poll(fresh, waitDur)
	}
	return nil
}

// refreshInstances re-queries GetInstances and returns the current view of the
// given instances (matched by InstanceID). Instances that disappeared are omitted.
func (s *b) refreshInstances(instances backends.InstanceList) (backends.InstanceList, error) {
	all, err := s.GetInstances(s.volumes, s.networks, s.firewalls)
	if err != nil {
		return nil, err
	}
	want := make(map[string]bool, len(instances))
	for _, inst := range instances {
		want[inst.InstanceID] = true
	}
	fresh := backends.InstanceList{}
	for _, inst := range all {
		if want[inst.InstanceID] {
			fresh = append(fresh, inst)
		}
	}
	return fresh, nil
}

// InstancesAddTags merges tags into each instance's node metadata only; no runner call
// is made (vagrant has no separate tagging mechanism, unlike cloud providers).
func (s *b) InstancesAddTags(instances backends.InstanceList, tags map[string]string) error {
	if len(instances) == 0 {
		return nil
	}
	for clusterName, insts := range groupInstancesByCluster(instances) {
		if err := s.withClusterLock(clusterName, func() error {
			meta, err := s.loadClusterMeta(clusterName)
			if err != nil {
				return err
			}
			if meta == nil {
				return nil
			}
			for _, inst := range insts {
				node := meta.Nodes[inst.NodeNo]
				if node == nil {
					continue
				}
				if node.Tags == nil {
					node.Tags = map[string]string{}
				}
				maps.Copy(node.Tags, tags)
			}
			return s.saveClusterMeta(meta)
		}); err != nil {
			return err
		}
	}
	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateInstance) //nolint:errcheck
	}
	return nil
}

// InstancesRemoveTags removes the given tag keys from each instance's node metadata only.
func (s *b) InstancesRemoveTags(instances backends.InstanceList, tagKeys []string) error {
	if len(instances) == 0 {
		return nil
	}
	for clusterName, insts := range groupInstancesByCluster(instances) {
		if err := s.withClusterLock(clusterName, func() error {
			meta, err := s.loadClusterMeta(clusterName)
			if err != nil {
				return err
			}
			if meta == nil {
				return nil
			}
			for _, inst := range insts {
				node := meta.Nodes[inst.NodeNo]
				if node == nil {
					continue
				}
				for _, k := range tagKeys {
					delete(node.Tags, k)
				}
			}
			return s.saveClusterMeta(meta)
		}); err != nil {
			return err
		}
	}
	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateInstance) //nolint:errcheck
	}
	return nil
}

// InstancesChangeExpiry is pure metadata: it sets Expires and the TAG_EXPIRES tag on
// each instance's node. (VolumesChangeExpiry remains stubbed until Phase 5.)
func (s *b) InstancesChangeExpiry(instances backends.InstanceList, expiry time.Time) error {
	if len(instances) == 0 {
		return nil
	}
	for clusterName, insts := range groupInstancesByCluster(instances) {
		if err := s.withClusterLock(clusterName, func() error {
			meta, err := s.loadClusterMeta(clusterName)
			if err != nil {
				return err
			}
			if meta == nil {
				return nil
			}
			for _, inst := range insts {
				node := meta.Nodes[inst.NodeNo]
				if node == nil {
					continue
				}
				node.Expires = expiry
				if expiry.IsZero() {
					delete(node.Tags, TAG_EXPIRES)
					continue
				}
				if node.Tags == nil {
					node.Tags = map[string]string{}
				}
				node.Tags[TAG_EXPIRES] = expiry.Format(time.RFC3339)
			}
			return s.saveClusterMeta(meta)
		}); err != nil {
			return err
		}
	}
	if s.invalidateCacheFunc != nil {
		s.invalidateCacheFunc(backends.CacheInvalidateInstance) //nolint:errcheck
	}
	return nil
}

// withClusterLock runs fn while holding the per-cluster lock for clusterName.
func (s *b) withClusterLock(clusterName string, fn func() error) error {
	lock := s.lockCluster(clusterName)
	lock.Lock()
	defer lock.Unlock()
	return fn()
}

// ResolveNetworkPlacement is a no-op for vagrant: there is a single fixed "local" zone
// and no VPC/subnet concept, so there's nothing to resolve.
func (s *b) ResolveNetworkPlacement(placement string) (vpc *backends.Network, subnet *backends.Subnet, zone string, err error) {
	return nil, nil, "", nil
}
