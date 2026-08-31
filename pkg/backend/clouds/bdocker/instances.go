package bdocker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/netip"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/sshkey"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/structtags"
	"github.com/charmbracelet/x/term"
	cerrdefs "github.com/containerd/errdefs"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid"
	"github.com/moby/moby/api/pkg/stdcopy"
	"github.com/moby/moby/api/types"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/image"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/api/types/network"
	"github.com/moby/moby/api/types/registry"
	"github.com/moby/moby/client"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

type CreateInstanceParams struct {
	// the image to use for the instances(nodes)
	Image *backends.Image `yaml:"image" json:"image" required:"true"`
	// specify the friendly-name of the docker server instance, followed by "," and the network name, e.g. docker-server,network1
	//
	// can specify 'default' as network name and 'default' as server name; can omit server name, in which case default will be used, and can omit network name, in which case the default network will be used
	//
	// ex: specify both: default,default ; omit server name: ,default ; omit network name: default, or leave empty to omit both
	NetworkPlacement string `yaml:"networkPlacement" json:"networkPlacement"`
	// volume types and sizes, backend-specific definitions
	//
	// docker format:
	//   {volumeName}:{mountTargetDirectory}
	//   example: volume1:/mnt/data
	//
	// used for mounting volumes to containers at startup
	Disks []string `yaml:"disks" json:"disks"`
	// optional: specify extra ports to expose and map. Acceptable formats:
	//   [+]{hostPort}:{containerPort} ; example: 8080:80 ; if the definition is prefixed with a +, the port will be mapped to the next available port (starting 8080)
	//
	//   host={hostIP:hostPORT},container={containerPORT},incr ; example: host=0.0.0.0:8080,container=80 ; incr parameter has same effect as the + prefix
	//
	//   [+]{hostIP:hostPORT},{containerPORT} ; example: 0.0.0.0:8080,80 ; if the definition is prefixed with a +, the port will be mapped to the next available port (starting 8080)
	// port 22 will be automatically mapped to the next unused port (starting 2200)
	Firewalls []string `yaml:"firewalls" json:"firewalls"`
	// --log-to-stderr - Will cause logging of all started services to be sent to stderr, this allows docker logs to view all service logs
	//
	// --no-logfile    - No journal logging
	//
	// --no-pidtrack   - Disable execve capture for cgroup-free PID tracking
	Cmd               []string            `yaml:"cmd" json:"cmd"`
	StopTimeout       *int                `yaml:"stopTimeout" json:"stopTimeout"` // seconds
	CapAdd            []string            `yaml:"capAdd" json:"capAdd"`
	CapDrop           []string            `yaml:"capDrop" json:"capDrop"`
	DNS               []string            `yaml:"dns" json:"dns"`
	DNSOptions        []string            `yaml:"dnsOptions" json:"dnsOptions"`
	DNSSearch         []string            `yaml:"dnsSearch" json:"dnsSearch"`
	Privileged        bool                `yaml:"privileged" json:"privileged"`
	SecurityOpt       []string            `yaml:"securityOpt" json:"securityOpt"`
	Tmpfs             map[string]string   `yaml:"tmpfs" json:"tmpfs"`
	RestartPolicy     string              `yaml:"restartPolicy" json:"restartPolicy"` // Always,None,OnFailure,UnlessStopped
	MaxRestartRetries int                 `yaml:"maxRestartRetries" json:"maxRestartRetries"`
	ShmSize           int64               `yaml:"shmSize" json:"shmSize"`
	Sysctls           map[string]string   `yaml:"sysctls" json:"sysctls"` // format: key=value of sysctl commands, like net.ipv4.ip_forward=1
	Resources         container.Resources `yaml:"resources" json:"resources"`
	MaskedPaths       []string            `yaml:"maskedPaths" json:"maskedPaths"`
	ReadonlyPaths     []string            `yaml:"readonlyPaths" json:"readonlyPaths"`
	SkipSshReadyCheck bool                `yaml:"skipSshReadyCheck" json:"skipSshReadyCheck"` // if set, will not test for ssh readiness
	// optional: registry authentication for pulling private images
	RegistryUser string `yaml:"registryUser" json:"registryUser"` // username for docker registry authentication
	RegistryPass string `yaml:"registryPass" json:"registryPass"` // password for docker registry authentication
	RegistryURL  string `yaml:"registryUrl" json:"registryUrl"`   // registry URL (e.g., docker.io, ghcr.io); if empty, uses default registry
}

type InstanceDetail struct {
	Docker container.Summary `json:"docker" yaml:"docker"`
}

// getInstanceDetail safely extracts *InstanceDetail from BackendSpecific, handling
// nil and map[string]interface{} (from JSON/YAML deserialization) gracefully.
func getInstanceDetail(inst *backends.Instance) *InstanceDetail {
	if inst.BackendSpecific == nil {
		inst.BackendSpecific = &InstanceDetail{}
		return inst.BackendSpecific.(*InstanceDetail)
	}
	if id, ok := inst.BackendSpecific.(*InstanceDetail); ok {
		return id
	}
	if m, ok := inst.BackendSpecific.(map[string]any); ok {
		jsonBytes, err := json.Marshal(m)
		if err == nil {
			var id InstanceDetail
			if err := json.Unmarshal(jsonBytes, &id); err == nil {
				inst.BackendSpecific = &id
				return &id
			}
		}
	}
	inst.BackendSpecific = &InstanceDetail{}
	return inst.BackendSpecific.(*InstanceDetail)
}

// getImageDetail safely extracts *ImageDetail from BackendSpecific, initializing it if needed.
// This handles cases where BackendSpecific might be nil, a map (from JSON/YAML deserialization),
// or already the correct type.
func getImageDetail(img *backends.Image) *ImageDetail {
	if img.BackendSpecific == nil {
		img.BackendSpecific = &ImageDetail{}
		return img.BackendSpecific.(*ImageDetail)
	}
	if id, ok := img.BackendSpecific.(*ImageDetail); ok {
		return id
	}
	// If it's a map (from JSON/YAML deserialization), try to convert it
	if m, ok := img.BackendSpecific.(map[string]any); ok {
		jsonBytes, err := json.Marshal(m)
		if err == nil {
			var id ImageDetail
			if err := json.Unmarshal(jsonBytes, &id); err == nil {
				img.BackendSpecific = &id
				return &id
			}
		}
	}
	// If conversion failed or it's something else, create a new ImageDetail
	img.BackendSpecific = &ImageDetail{}
	return img.BackendSpecific.(*ImageDetail)
}

// lifeCycleState maps a Docker/Podman container state string onto the aerolab
// lifecycle state. Unrecognised states report as running, which is the historic
// behaviour of this mapping.
func lifeCycleState(state container.ContainerState) backends.LifeCycleState {
	switch state {
	case "exited":
		return backends.LifeCycleStateStopped
	case "dead":
		return backends.LifeCycleStateFail
	case "paused":
		return backends.LifeCycleStateUnknown
	case "restarting":
		return backends.LifeCycleStateStarting
	case "created":
		return backends.LifeCycleStateCreated
	}
	return backends.LifeCycleStateRunning
}

func (s *b) GetInstances(volumes backends.VolumeList, networkList backends.NetworkList, firewallList backends.FirewallList) (backends.InstanceList, error) {
	log := s.log.WithPrefix("GetInstances: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	var i backends.InstanceList
	ilock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()
	wg.Add(len(zones))
	var errs error
	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)
			cli, err := s.getDockerClient(zone)
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}

			f := make(client.Filters)
			if !s.listAllProjects {
				f.Add("label", TAG_AEROLAB_PROJECT+"="+s.project)
			}
			containers, err := cli.ContainerList(context.Background(), client.ContainerListOptions{
				Size:    true,
				All:     true,
				Filters: f,
			})
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			for _, container := range containers.Items {
				if container.Labels[TAG_AEROLAB_VERSION] == "" {
					continue
				}
				nodeNo, _ := strconv.Atoi(container.Labels[TAG_NODE_NO])
				var arch backends.Architecture
				arch.FromString(container.Labels[TAG_ARCHITECTURE]) //nolint:errcheck
				name := container.ID
				if len(container.Names) > 0 {
					name = strings.TrimPrefix(container.Names[0], "/")
				}
				createTime := time.Time{}
				if container.Created != 0 {
					createTime = time.Unix(container.Created, 0)
				}
				expires := time.Time{}
				if val, ok := container.Labels[TAG_EXPIRES]; ok {
					expires, _ = time.Parse(time.RFC3339, val)
				}
				var net *network.EndpointSettings
				if container.NetworkSettings != nil {
					for _, network := range container.NetworkSettings.Networks {
						net = network
						break
					}
				}
				netID := ""
				if net != nil {
					netID = net.NetworkID
				}
				network := networkList.WithNetID(netID)
				subnetID := ""
				if network.Count() > 0 {
					subnets := network.Subnets()
					if len(subnets) > 0 {
						subnetID = subnets[0].SubnetId
					}
				}
				ip := ""
				if net != nil && net.IPAddress.IsValid() {
					ip = net.IPAddress.String()
				}
				istate := lifeCycleState(container.State)
				fw := []string{}
				for _, port := range container.Ports {
					// fw format: host={hostIP:hostPORT},container={containerPORT} ; example: host=0.0.0.0:8080,container=80
					hostIP := ""
					if port.IP.IsValid() {
						hostIP = port.IP.String()
					}
					fw = append(fw, fmt.Sprintf("host=%s:%d,container=%d", hostIP, port.PublicPort, port.PrivatePort))
				}
				// Compute AccessURL based on client type and port mappings
				accessURL := computeAccessURL(container.Labels["aerolab.client.type"], container.Ports)

				ilock.Lock()
				i = append(i, &backends.Instance{
					ClusterName: container.Labels[TAG_CLUSTER_NAME],
					ClusterUUID: container.Labels[TAG_CLUSTER_UUID],
					NodeNo:      nodeNo,
					IP: backends.IP{
						Private: ip,
					},
					ImageID:      container.ImageID,
					SubnetID:     subnetID,
					NetworkID:    netID,
					Architecture: arch,
					OperatingSystem: backends.OS{
						Name:    container.Labels[TAG_OS_NAME],
						Version: container.Labels[TAG_OS_VERSION],
					},
					Firewalls:        fw,
					InstanceID:       container.ID,
					BackendType:      backends.BackendTypeDocker,
					InstanceType:     "", // unused since docker is not a cloud provider
					SpotInstance:     false,
					Name:             name,
					ZoneName:         zone,
					ZoneID:           zone,
					CreationTime:     createTime,
					EstimatedCostUSD: backends.Cost{},
					AttachedVolumes:  nil,
					Owner:            container.Labels[TAG_OWNER],
					InstanceState:    istate,
					Tags:             container.Labels,
					Expires:          expires,
					Description:      container.Labels[TAG_DESCRIPTION],
					CustomDNS:        nil,
					AccessURL:        accessURL,
					BackendSpecific: &InstanceDetail{
						Docker: container,
					},
				})
				ilock.Unlock()
			}
		}(zone)
	}
	wg.Wait()
	if errs == nil {
		s.instances = i
		s.usedPorts.reset(s.instances)
	}
	return i, errs
}

func (s *b) InstancesAddTags(instances backends.InstanceList, tags map[string]string) error {
	log := s.log.WithPrefix("InstancesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	return errors.New("not implemented")
}

func (s *b) InstancesRemoveTags(instances backends.InstanceList, tagKeys []string) error {
	log := s.log.WithPrefix("InstancesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	return errors.New("not implemented")
}

func (s *b) InstancesTerminate(instances backends.InstanceList, waitDur time.Duration) error {
	stopErr := s.InstancesStop(instances.WithState(backends.LifeCycleStateRunning).Describe(), true, 0)
	if stopErr != nil {
		return stopErr
	}
	log := s.log.WithPrefix("InstancesTerminate: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}

	// DISABLED: SSH key auto-deletion on terminate.
	// The inventory (s.instances) may be a filtered subset, and the key is shared across all
	// backends/regions. Incorrectly concluding all instances are gone deletes the local key
	// files, orphaning instances that still exist elsewhere.
	// The key files are ~2KB per project — not worth the risk of premature deletion.
	// removeSSHKey := s.instances.WithBackendType(backends.BackendTypeDocker).WithNotState(backends.LifeCycleStateTerminating, backends.LifeCycleStateTerminated).Count() == instances.Count()

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance) //nolint:errcheck
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)   //nolint:errcheck

	// Forget the remembered SSH host keys: the identity is reusable, so a new
	// container created later with the same cluster UUID and node number must
	// be learned afresh rather than inherit a stale key.
	hostKeyIDs := make([]string, 0, len(instances))
	for _, instance := range instances {
		if id := instance.HostKeyID(); id != "" {
			hostKeyIDs = append(hostKeyIDs, id)
		}
	}
	s.forgetHostKeys(hostKeyIDs...)

	instanceIds := make(map[string][]string)
	for _, instance := range instances {
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []string{}
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance.InstanceID)
	}

	for zone, ids := range instanceIds {
		log.Detail("zone=%s start", zone)
		defer log.Detail("zone=%s end", zone)
		cli, err := s.getDockerClient(zone)
		if err != nil {
			return err
		}
		for _, id := range ids {
			wg := new(sync.WaitGroup)
			wg.Add(1)
			var reterr error
			go func(id string) {
				defer wg.Done()
				log.Detail("removing container %s", id)
				_, err := cli.ContainerRemove(context.Background(), id, client.ContainerRemoveOptions{
					Force: true,
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
				}
			}(id)
			wg.Wait()
			if reterr != nil {
				return reterr
			}
		}
	}

	// DISABLED: see comment at top of InstancesTerminate about SSH key auto-deletion.
	/*
		if removeSSHKey && s.createInstanceCount.Get() == 0 {
			log.Detail("Remove SSH keys as no more instances exist for this project")
			os.Remove(filepath.Join(s.sshKeysDir, s.project))
			os.Remove(filepath.Join(s.sshKeysDir, s.project+".pub"))
			log.Detail("SSH keys removed")
		}
	*/
	return nil
}

func (s *b) InstancesStop(instances backends.InstanceList, force bool, waitDur time.Duration) error {
	log := s.log.WithPrefix("InstancesStop: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance) //nolint:errcheck
	instanceIds := make(map[string][]string)
	for _, instance := range instances {
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []string{}
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance.InstanceID)
	}
	for zone, ids := range instanceIds {
		log.Detail("zone=%s start", zone)
		defer log.Detail("zone=%s end", zone)
		cli, err := s.getDockerClient(zone)
		if err != nil {
			return err
		}
		for _, id := range ids {
			wg := new(sync.WaitGroup)
			wg.Add(1)
			var reterr error
			go func(id string) {
				defer wg.Done()
				log.Detail("stopping container %s", id)
				timeout := int(waitDur.Seconds())
				_, err := cli.ContainerStop(context.Background(), id, client.ContainerStopOptions{
					Signal:  "SIGTERM",
					Timeout: &timeout,
				})
				if err != nil {
					reterr = errors.Join(reterr, err)
				}
			}(id)
			wg.Wait()
			if reterr != nil {
				return reterr
			}
		}
		if waitDur > 0 {
			for _, id := range ids {
				for {
					inspected, err := cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
					if err != nil {
						return err
					}
					if !inspected.Container.State.Running {
						break
					}
					time.Sleep(250 * time.Millisecond)
				}
			}
		}
	}
	return nil
}

func (s *b) InstancesStart(instances backends.InstanceList, waitDur time.Duration) error {
	log := s.log.WithPrefix("InstancesStart: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	startedAt := time.Now()
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance) //nolint:errcheck
	instanceIds := make(map[string][]string)
	for _, instance := range instances {
		if _, ok := instanceIds[instance.ZoneID]; !ok {
			instanceIds[instance.ZoneID] = []string{}
		}
		instanceIds[instance.ZoneID] = append(instanceIds[instance.ZoneID], instance.InstanceID)
	}
	for zone, ids := range instanceIds {
		log.Detail("zone=%s start", zone)
		defer log.Detail("zone=%s end", zone)
		cli, err := s.getDockerClient(zone)
		if err != nil {
			return err
		}
		for _, id := range ids {
			log.Detail("starting container %s", id)
			if _, err := cli.ContainerStart(context.Background(), id, client.ContainerStartOptions{}); err != nil {
				return err
			}
		}
		if err := s.waitForContainersRunning(cli, ids, containerRunningBudget, log); err != nil {
			return fmt.Errorf("%w%s", err, s.diagnoseStopped(cli, ids))
		}
		// A stopped container reports no published ports: the Docker API only
		// populates NetworkSettings.Ports (and thus container.Summary.Ports)
		// while the container is running. The instance structs handed to us were
		// built from the pre-start inventory, so their cached summary has an
		// empty port list and a possibly-stale IP. Re-list the just-started
		// containers and refresh each instance's cached summary/IP. Without this,
		// the ssh-ready probe below (and any exec that runs before the next
		// inventory refresh) resolves SSH's host port to 0 and can never connect.
		if err := s.refreshStartedInstances(cli, ids, instances); err != nil {
			return err
		}
	}

	// Docker reports the container as Running but sshd / docker-exec may not be
	// ready yet. Probe with `ls /` until all instances respond or the budget
	// runs out. Match the bgcp / baws behaviour so callers (cluster.start ->
	// FixMesh, client.start -> SFTP) can assume SSH works on return.
	if waitDur > 0 {
		for _, inst := range instances {
			inst.InstanceState = backends.LifeCycleStateRunning
		}
		remaining := waitDur - time.Since(startedAt)
		log.Detail("Waiting for instances to be ssh-ready (budget: %s)", remaining)
		if !s.waitForSSHReady(instances, remaining, log) {
			return fmt.Errorf("instances started but failed to become ssh-ready within %s%s", waitDur, s.describeInstanceContainers(instanceIds))
		}
	}
	return nil
}

// containerRunningBudget bounds how long we wait for a started container to
// report a running state. Reaching running is a local operation; anything
// slower than this means the container is not coming up at all.
const containerRunningBudget = 2 * time.Minute

// containerInspector is the slice of the docker client that
// waitForContainersRunning needs, so the wait can be tested without a daemon.
type containerInspector interface {
	ContainerInspect(ctx context.Context, containerID string, options client.ContainerInspectOptions) (client.ContainerInspectResult, error)
}

// waitForContainersRunning blocks until every container in ids reports a
// running state, or returns an error if one of them dies or the budget runs
// out. Starting a container is not enough to make it usable: both Docker and
// Podman publish port mappings and network settings only once the container is
// running, so anything that reads the container list before this returns can
// see an empty port list. Podman on a Windows/WSL2 host is markedly slower to
// settle here than a native Linux daemon.
func (s *b) waitForContainersRunning(cli containerInspector, ids []string, budget time.Duration, log loggerIface) error {
	deadline := time.Now().Add(budget)
	for _, id := range ids {
		log.Detail("waiting for container %s to be running", id)
		for {
			inspected, err := cli.ContainerInspect(context.Background(), id, client.ContainerInspectOptions{})
			if err != nil {
				// A container created with auto-remove that dies on startup is
				// gone by the time we look, so say what happened rather than
				// reporting a bare "no such container" for an ID we just made.
				if cerrdefs.IsNotFound(err) {
					return fmt.Errorf("container %s disappeared before it reached running state; it most likely exited on startup and was auto-removed", id)
				}
				return fmt.Errorf("failed to inspect container %s: %w", id, err)
			}
			state := inspected.Container.State
			if state == nil {
				return fmt.Errorf("container %s reported no state", id)
			}
			if state.Running {
				break
			}
			// A container that already exited is never going to come back on
			// its own, so fail immediately with the exit code instead of
			// burning the whole budget. This is the usual shape of an init
			// system that cannot start inside the container.
			if state.Status == "exited" || state.Status == "dead" {
				msg := fmt.Sprintf("container %s is %s (exit code %d) instead of running", id, state.Status, state.ExitCode)
				if state.Error != "" {
					msg += ": " + state.Error
				}
				return errors.New(msg)
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("container %s did not reach running state within %s (state=%s)", id, budget, state.Status)
			}
			time.Sleep(250 * time.Millisecond)
		}
		log.Detail("container %s is running", id)
	}
	return nil
}

// refreshStartedInstances re-lists the containers in a zone and refreshes the
// cached container summary (state, published ports and private IP) on each
// instance whose ID is in ids. This is required after starting a container
// because Docker only reports published ports and network settings for running
// containers; the summary captured while the container was stopped has an empty
// port list, which would otherwise cause SSH connections to resolve to host
// port 0.
func (s *b) refreshStartedInstances(cli *client.Client, ids []string, instances backends.InstanceList) error {
	f := make(client.Filters)
	if !s.listAllProjects {
		f.Add("label", TAG_AEROLAB_PROJECT+"="+s.project)
	}
	fresh, err := cli.ContainerList(context.Background(), client.ContainerListOptions{
		Size:    true,
		All:     true,
		Filters: f,
	})
	if err != nil {
		return err
	}
	byID := make(map[string]container.Summary, len(fresh.Items))
	for _, c := range fresh.Items {
		byID[c.ID] = c
	}
	idSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		idSet[id] = struct{}{}
	}
	for _, inst := range instances {
		if _, ok := idSet[inst.InstanceID]; !ok {
			continue
		}
		c, ok := byID[inst.InstanceID]
		if !ok {
			continue
		}
		getInstanceDetail(inst).Docker = c
		inst.InstanceState = lifeCycleState(c.State)
		if c.NetworkSettings != nil {
			for _, n := range c.NetworkSettings.Networks {
				if n != nil && n.IPAddress.IsValid() {
					inst.IP.Private = n.IPAddress.String()
					break
				}
			}
		}
	}
	return nil
}

// waitForSSHReady polls each instance via a lightweight `ls /` exec until all
// respond successfully or the budget runs out. Returns true when every instance
// answered at least once within the budget; false on timeout.
func (s *b) waitForSSHReady(instances backends.InstanceList, budget time.Duration, log loggerIface) bool {
	if budget <= 0 {
		return false
	}
	parallel := len(instances)
	if parallel <= 0 {
		return true
	}
	remaining := budget
	for remaining > 0 {
		iterStart := time.Now()
		out := s.InstancesExec(instances, &backends.ExecInput{
			Username:        "root",
			ParallelThreads: parallel,
			ConnectTimeout:  5 * time.Second,
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"ls", "/"},
				SessionTimeout: 10 * time.Second,
			},
		})
		success := len(out) == len(instances)
		for _, o := range out {
			if o.Output.Err != nil {
				success = false
				log.Detail("Waiting for instance %s to be ssh-ready: %s", o.Instance.InstanceID, o.Output.Err)
			}
		}
		if success {
			return true
		}
		remaining -= time.Since(iterStart)
		if remaining > 0 {
			time.Sleep(1 * time.Second)
			remaining -= 1 * time.Second
		}
	}
	return false
}

// loggerIface lets waitForSSHReady stay decoupled from the concrete logger
// type; we only need Detail(format, args...) in this hot path.
type loggerIface interface {
	Detail(format string, args ...any)
}

// sshHostPort resolves the host port that the instance's container port 22 is
// published on. Aerolab always reaches containers over loopback on that
// published port, never on the container's own IP, so an unresolved port is a
// hard failure rather than something a retry can fix: dialling port 0 would
// fail identically on every attempt until the caller's budget expired.
func sshHostPort(i *backends.Instance) (int, error) {
	for _, x := range getInstanceDetail(i).Docker.Ports {
		if x.PrivatePort == 22 {
			if x.PublicPort == 0 {
				return 0, fmt.Errorf("container %s exposes port 22 but it is not published on a host port; the container may not be running yet", i.InstanceID)
			}
			return int(x.PublicPort), nil
		}
	}
	return 0, fmt.Errorf("container %s has no host port mapped to port 22 (host->container ports: %s)", i.InstanceID, describePorts(getInstanceDetail(i).Docker.Ports))
}

// unresolvedSSHTargets reports whether any instance still lacks the details
// needed to attempt an SSH connection, meaning its cached container summary is
// worth re-reading.
func unresolvedSSHTargets(instances backends.InstanceList) bool {
	for _, i := range instances {
		if i.InstanceState != backends.LifeCycleStateRunning {
			return true
		}
		if _, err := sshHostPort(i); err != nil {
			return true
		}
	}
	return false
}

// describePorts renders a container's published ports for error messages.
func describePorts(ports []container.PortSummary) string {
	if len(ports) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(ports))
	for _, p := range ports {
		parts = append(parts, fmt.Sprintf("%d->%d/%s", p.PublicPort, p.PrivatePort, p.Type))
	}
	return strings.Join(parts, ",")
}

func (s *b) InstancesExec(instances backends.InstanceList, e *backends.ExecInput) []*backends.ExecOutput {
	log := s.log.WithPrefix("InstancesExecSSH: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	if e.ParallelThreads == 0 {
		e.ParallelThreads = len(instances)
	}
	out := []*backends.ExecOutput{}
	outl := new(sync.Mutex)
	parallelize.ForEachLimit(instances, e.ParallelThreads, func(i *backends.Instance) {
		if i.InstanceState != backends.LifeCycleStateRunning {
			outl.Lock()
			out = append(out, &backends.ExecOutput{
				Output: &sshexec.ExecOutput{
					Err: errors.New("instance not running"),
				},
				Instance: i,
			})
			outl.Unlock()
			return
		}
		// Determine whether to use docker exec or SSH.
		// Use docker exec only for true custom images (no systemd/SSH).
		// If port 22 is mapped, the instance was built from a systemd image
		// and should always use SSH, even if incorrectly tagged as custom.
		useDockerExec := false
		if d, ok := i.Tags["aerolab.custom.image"]; ok && d == "true" {
			useDockerExec = true
			for _, x := range getInstanceDetail(i).Docker.Ports {
				if x.PrivatePort == 22 {
					useDockerExec = false
					break
				}
			}
		}
		if useDockerExec {
			cli, err := s.getDockerClient(i.ZoneName)
			if err != nil {
				outl.Lock()
				out = append(out, &backends.ExecOutput{
					Output: &sshexec.ExecOutput{
						Err: err,
					},
					Instance: i,
				})
				outl.Unlock()
				return
			}
			env := []string{}
			for _, x := range e.Env {
				env = append(env, x.Key+"="+x.Value)
			}
			env = append(env, "AEROLAB_CLUSTER_NAME="+i.ClusterName)
			env = append(env, "AEROLAB_NODE_NO="+strconv.Itoa(i.NodeNo))
			env = append(env, "AEROLAB_PROJECT_NAME="+s.project)
			env = append(env, "AEROLAB_OWNER="+i.Owner)
			cmd := e.Command
			if len(cmd) == 0 {
				cmd = []string{"/bin/bash"}
			}
			sout := e.Stdout
			serr := e.Stderr
			var stdout, stderr bytes.Buffer
			if sout == nil {
				sout = &stdout
			}
			if serr == nil {
				serr = &stderr
			}
			retCode, err := ExecWithCLI(context.Background(), cli, i.InstanceID, cmd, env, e.Stdin, sout, serr, e.Terminal)
			if retCode != 0 || err != nil {
				err = errors.Join(err, fmt.Errorf("exec failed with exit code %d", retCode))
			}
			outl.Lock()
			out = append(out, &backends.ExecOutput{
				Output: &sshexec.ExecOutput{
					Stdout: stdout.Bytes(),
					Stderr: stderr.Bytes(),
					Err:    err,
				},
				Instance: i,
			})
			outl.Unlock()
			return
		} else {
			nKey, err := os.ReadFile(path.Join(s.sshKeysDir, s.project))
			if err != nil {
				outl.Lock()
				out = append(out, &backends.ExecOutput{
					Output: &sshexec.ExecOutput{
						Err: err,
					},
					Instance: i,
				})
				outl.Unlock()
				return
			}
			sshPort, err := sshHostPort(i)
			if err != nil {
				outl.Lock()
				out = append(out, &backends.ExecOutput{
					Output: &sshexec.ExecOutput{
						Err: err,
					},
					Instance: i,
				})
				outl.Unlock()
				return
			}
			clientConf := sshexec.ClientConf{
				Host:           "127.0.0.1",
				Port:           sshPort,
				Username:       e.Username,
				PrivateKey:     nKey,
				ConnectTimeout: e.ConnectTimeout,
				MaxRetries:     e.MaxRetries,
				RetrySleep:     e.RetrySleep,
			}
			s.applyHostKeyPolicy(&clientConf, i)
			execInput := &sshexec.ExecInput{
				ClientConf: clientConf,
				ExecDetail: e.ExecDetail,
			}
			execInput.Env = append(execInput.Env, &sshexec.Env{
				Key:   "AEROLAB_CLUSTER_NAME",
				Value: i.ClusterName,
			})
			execInput.Env = append(execInput.Env, &sshexec.Env{
				Key:   "AEROLAB_NODE_NO",
				Value: strconv.Itoa(i.NodeNo),
			})
			execInput.Env = append(execInput.Env, &sshexec.Env{
				Key:   "AEROLAB_PROJECT_NAME",
				Value: s.project,
			})
			execInput.Env = append(execInput.Env, &sshexec.Env{
				Key:   "AEROLAB_OWNER",
				Value: i.Owner,
			})
			o := sshexec.ExecWithRetry(execInput, "ssh-exec-"+i.InstanceID)
			outl.Lock()
			out = append(out, &backends.ExecOutput{
				Output:   o,
				Instance: i,
			})
			outl.Unlock()
		}
	})
	return out
}

func (s *b) InstancesGetSSHKeyPath(instances backends.InstanceList) []string {
	log := s.log.WithPrefix("InstancesGetSSHKeyPath: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	out := []string{}
	for range instances {
		out = append(out, path.Join(s.sshKeysDir, s.project))
	}
	return out
}

func (s *b) InstancesGetSftpConfig(instances backends.InstanceList, username string) ([]*sshexec.ClientConf, error) {
	log := s.log.WithPrefix("InstancesGetSftpConfig: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	confs := []*sshexec.ClientConf{}
	for _, i := range instances {
		if i.InstanceState != backends.LifeCycleStateRunning {
			return nil, errors.New("instance not running")
		}
		nKey, err := os.ReadFile(path.Join(s.sshKeysDir, s.project))
		if err != nil {
			return nil, errors.New("required key not found")
		}
		sshPort, err := sshHostPort(i)
		if err != nil {
			return nil, err
		}
		clientConf := &sshexec.ClientConf{
			Host:           "127.0.0.1",
			Port:           sshPort,
			Username:       username,
			PrivateKey:     nKey,
			ConnectTimeout: 30 * time.Second,
		}
		s.applyHostKeyPolicy(clientConf, i)
		confs = append(confs, clientConf)
	}
	return confs, nil
}

func (s *b) InstancesAssignFirewalls(instances backends.InstanceList, fw backends.FirewallList) error {
	log := s.log.WithPrefix("InstancesAssignFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	return errors.New("not implemented")
}

func (s *b) InstancesRemoveFirewalls(instances backends.InstanceList, fw backends.FirewallList) error {
	// checked before touching s.log: this backend is registered at init time and
	// only initialized if it is enabled, so a no-instance call can arrive on a
	// zero-value receiver whose logger is still nil.
	if len(instances) == 0 {
		return nil
	}
	log := s.log.WithPrefix("InstancesRemoveFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	return errors.New("not implemented")
}

// on docker, the cost is always 0
func (s *b) CreateInstancesGetPrice(input *backends.CreateInstanceInput) (costPPH, costGB float64, err error) {
	return 0, 0, nil
}

type dockerBuilder struct {
	docker *image.Summary
	wg     *sync.WaitGroup
}

func (s *b) CreateInstances(input *backends.CreateInstanceInput, waitDur time.Duration) (output *backends.CreateInstanceOutput, err error) {
	// resolve network placement using s.networks, so we have VPC, Subnet and Zone from it, user provided either vpc- or subnet- or zone name
	log := s.log.WithPrefix("CreateInstances: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	s.createInstanceCount.Inc()
	defer s.createInstanceCount.Dec()

	// resolve backend-specific parameters
	backendSpecificParams := &CreateInstanceParams{}
	if input.BackendSpecificParams != nil {
		if _, ok := input.BackendSpecificParams[backends.BackendTypeDocker]; ok {
			switch input.BackendSpecificParams[backends.BackendTypeDocker].(type) {
			case *CreateInstanceParams:
				backendSpecificParams = input.BackendSpecificParams[backends.BackendTypeDocker].(*CreateInstanceParams)
			case CreateInstanceParams:
				item := input.BackendSpecificParams[backends.BackendTypeDocker].(CreateInstanceParams)
				backendSpecificParams = &item
			default:
				return nil, fmt.Errorf("invalid backend-specific parameters for docker")
			}
		}
	}
	if err := structtags.CheckRequired(backendSpecificParams); err != nil {
		return nil, fmt.Errorf("required fields missing in backend-specific parameters: %w", err)
	}
	// if cluster with given ClusterName already exists in s.instances, find last node number, so we know where to count up for the instances we will be creating
	lastNodeNo := 0
	clusterUUID := uuid.New().String()
	for _, instance := range s.instances.WithNotState(backends.LifeCycleStateTerminated).WithClusterName(input.ClusterName).Describe() {
		clusterUUID = instance.ClusterUUID
		if instance.NodeNo > lastNodeNo {
			lastNodeNo = instance.NodeNo
		}
	}
	log.Detail("Current last node number in cluster %s: %d", input.ClusterName, lastNodeNo)

	// Drop any host key still remembered for the node numbers we are about to
	// occupy. Terminate already clears these, but a container removed outside
	// AeroLab (or a store from an older version) would look like a mismatch.
	newHostKeyIDs := make([]string, 0, input.Nodes)
	for n := lastNodeNo + 1; n <= lastNodeNo+input.Nodes; n++ {
		if id := backends.HostKeyID(backends.BackendTypeDocker, clusterUUID, n); id != "" {
			newHostKeyIDs = append(newHostKeyIDs, id)
		}
	}
	s.forgetHostKeys(newHostKeyIDs...)

	// create docker tags for docker.CreateInstancesInput
	labels := map[string]string{
		TAG_OWNER:           input.Owner,
		TAG_CLUSTER_NAME:    input.ClusterName,
		TAG_DESCRIPTION:     input.Description,
		TAG_AEROLAB_PROJECT: s.project,
		TAG_AEROLAB_VERSION: s.aerolabVersion,
		TAG_OS_NAME:         backendSpecificParams.Image.OSName,
		TAG_OS_VERSION:      backendSpecificParams.Image.OSVersion,
		TAG_CLUSTER_UUID:    clusterUUID,
	}
	// Only add expiry tag if a non-zero expiry time is set
	if !input.Expires.IsZero() {
		labels[TAG_EXPIRES] = input.Expires.Format(time.RFC3339)
	}
	maps.Copy(labels, input.Tags)

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
	// connect
	serverName := "default"
	networkName := "bridge"
	if backendSpecificParams.NetworkPlacement != "" {
		split := strings.Split(backendSpecificParams.NetworkPlacement, ",")
		if len(split) > 0 {
			serverName = split[0]
		}
		if len(split) > 1 {
			networkName = split[1]
		}
	}
	endpoints := map[string]*network.EndpointSettings{}
	if networkName != "bridge" {
		endpoints[networkName] = &network.EndpointSettings{
			Aliases: []string{},
		}
	}

	cli, err := s.getDockerClient(serverName)
	if err != nil {
		return nil, err
	}

	// resolve SSHKeyName
	publicKeyBytes, err := sshkey.Ensure(s.sshKeysDir, s.project, log)
	if err != nil {
		return nil, err
	}

	// if image is public, check if we have a custom build already; if not: make one
	// we need to track who is building the image, so that if another CreateInstances is already building this particular image, we should just wait for it to finish
	imgName := backendSpecificParams.Image.Name
	if backendSpecificParams.Image.Public {
		imgDetail := getImageDetail(backendSpecificParams.Image)
		if imgDetail.Docker == nil {
			s.builderMutex.Lock()
			if _, ok := s.builders[backendSpecificParams.Image.ZoneName]; !ok {
				s.builders[backendSpecificParams.Image.ZoneName] = make(map[string]*dockerBuilder)
			}
			if builder, ok := s.builders[backendSpecificParams.Image.ZoneName][backendSpecificParams.Image.Name]; ok {
				builder.wg.Wait()
				if builder.docker != nil {
					reassign := getImageDetail(backendSpecificParams.Image)
					reassign.Docker = builder.docker
					s.builderMutex.Unlock()
				} else {
					imgName = ""
				}
			} else {
				imgName = ""
			}
			if imgName == "" {
				s.builders[backendSpecificParams.Image.ZoneName][backendSpecificParams.Image.Name] = &dockerBuilder{
					docker: nil,
					wg:     new(sync.WaitGroup),
				}
				s.builders[backendSpecificParams.Image.ZoneName][backendSpecificParams.Image.Name].wg.Add(1)
				s.builderMutex.Unlock()
				err = func() error {
					defer s.builders[backendSpecificParams.Image.ZoneName][backendSpecificParams.Image.Name].wg.Done()
					imgLabels := map[string]string{
						TAG_AEROLAB_VERSION: s.aerolabVersion,
						TAG_OS_NAME:         backendSpecificParams.Image.OSName,
						TAG_OS_VERSION:      backendSpecificParams.Image.OSVersion,
						TAG_PUBLIC_NAME:     backendSpecificParams.Image.Name,
						TAG_PUBLIC_TEMPLATE: "true",
						TAG_ARCHITECTURE:    backendSpecificParams.Image.Architecture.String(),
					}
					// create a new image using docker build process, and assign it to the image variable, ensure the correct tags are set TAG_PUBLIC_NAME,TAG_PUBLIC_TEMPLATE, OS_NAME, OS_VERSION, ARCHITECTURE
					df, err := scripts.ReadFile("scripts/Dockerfile")
					if err != nil {
						return fmt.Errorf("failed to read Dockerfile: %v", err)
					}
					ud, err := scripts.ReadFile("scripts/userdata.sh")
					if err != nil {
						return fmt.Errorf("failed to read userdata.sh: %v", err)
					}
					df = fmt.Appendf(nil, string(df), backendSpecificParams.Image.Name)
					buf := new(bytes.Buffer)
					tw := tar.NewWriter(buf)
					if err := tw.WriteHeader(&tar.Header{
						Name: "Dockerfile",
						Mode: 0644,
						Size: int64(len(df)),
					}); err != nil {
						return fmt.Errorf("tar write header: %w", err)
					}
					if _, err := tw.Write(df); err != nil {
						return fmt.Errorf("tar write Dockerfile: %w", err)
					}
					if err := tw.WriteHeader(&tar.Header{
						Name: "userdata.sh",
						Mode: 0755,
						Size: int64(len(ud)),
					}); err != nil {
						return fmt.Errorf("tar write header: %w", err)
					}
					if _, err := tw.Write(ud); err != nil {
						return fmt.Errorf("tar write userdata: %w", err)
					}
					tw.Flush() //nolint:errcheck
					tw.Close() //nolint:errcheck
					pf := v1.Platform{OS: "linux", Architecture: "amd64"}
					switch backendSpecificParams.Image.Architecture {
					case backends.ArchitectureARM64:
						pf.Architecture = "arm64"
					case backends.ArchitectureNative:
						if runtime.GOARCH == "arm64" {
							pf.Architecture = "arm64"
						}
					}
					newNameTag := backendSpecificParams.Image.Architecture.String() + "-" + backendSpecificParams.Image.OSName + "-" + backendSpecificParams.Image.OSVersion
					if s.isPodman[backendSpecificParams.Image.ZoneName] {
						newNameTag = "localhost/" + newNameTag
					}
					builder, err := cli.ImageBuild(context.Background(), buf, client.ImageBuildOptions{
						Tags: []string{
							newNameTag,
						},
						SuppressOutput: false, // sshh, do it quietly
						Remove:         true,  // always remove image
						ForceRemove:    true,  // always remove image
						PullParent:     true,  // always pull latest parent image (public one)
						Dockerfile:     "",
						Labels:         imgLabels,
						Squash:         false,
						Platforms:      []v1.Platform{pf},
						Outputs:        []client.ImageBuildOutput{},
					})
					if err != nil {
						return fmt.Errorf("failed to build image: %v", err)
					}
					defer builder.Body.Close()

					type BuildLine struct {
						Stream      string `json:"stream"`
						Error       string `json:"error"`
						ErrorDetail struct {
							Message string `json:"message"`
						} `json:"errorDetail"`
						Aux struct {
							ID string `json:"ID"`
						} `json:"aux"`
					}
					scanner := bufio.NewScanner(builder.Body)
					for scanner.Scan() {
						var line BuildLine
						if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
							return fmt.Errorf("failed to unmarshal docker build response (%s): %v", scanner.Text(), err)
						}

						if line.Error != "" {
							return fmt.Errorf("docker build failed: %s: %s", line.Error, line.ErrorDetail.Message)
						}

						if line.Stream != "" {
							log.Detail("DOCKER-BUILD: %s", line.Stream)
						}
					}
					if err := scanner.Err(); err != nil {
						return fmt.Errorf("failed to read docker build response: %v", err)
					}

					reassign := getImageDetail(backendSpecificParams.Image)
					reassign.Docker = &image.Summary{
						Created:  time.Now().Unix(),
						ID:       newNameTag,
						Labels:   imgLabels,
						ParentID: backendSpecificParams.Image.Name,
						RepoTags: []string{newNameTag},
					}
					s.builders[backendSpecificParams.Image.ZoneName][backendSpecificParams.Image.Name].docker = reassign.Docker
					return nil
				}()
				if err != nil {
					return nil, err
				}
			}
		}
		imgDetail = getImageDetail(backendSpecificParams.Image)
		if imgDetail.Docker != nil {
			if len(imgDetail.Docker.RepoTags) > 0 {
				imgName = imgDetail.Docker.RepoTags[0]
			} else {
				imgName = imgDetail.Docker.ID
			}
		}
	}

	// Pull custom image with authentication if credentials are provided
	if !backendSpecificParams.Image.Public && backendSpecificParams.RegistryUser != "" && backendSpecificParams.RegistryPass != "" {
		log.Detail("Pulling image %s with registry authentication", imgName)
		pullOpts := client.ImagePullOptions{}

		// Create auth config
		authConfig := registry.AuthConfig{
			Username:      backendSpecificParams.RegistryUser,
			Password:      backendSpecificParams.RegistryPass,
			ServerAddress: backendSpecificParams.RegistryURL,
		}
		encodedAuth, err := encodeAuthToBase64(authConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to encode registry auth: %w", err)
		}
		pullOpts.RegistryAuth = encodedAuth

		// Pull the image
		reader, err := cli.ImagePull(context.Background(), imgName, pullOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to pull image %s: %w", imgName, err)
		}
		defer reader.Close()

		// Read the output to ensure pull completes
		_, err = io.Copy(io.Discard, reader)
		if err != nil {
			return nil, fmt.Errorf("failed to pull image %s: %w", imgName, err)
		}
		log.Detail("Successfully pulled image %s", imgName)
	} else if !backendSpecificParams.Image.Public {
		// For custom images without auth, try to pull (will use local docker credentials if configured)
		log.Detail("Pulling image %s (using local docker credentials if configured)", imgName)
		reader, err := cli.ImagePull(context.Background(), imgName, client.ImagePullOptions{})
		if err != nil {
			// If pull fails, check if image exists locally
			_, inspectErr := cli.ImageInspect(context.Background(), imgName)
			if inspectErr != nil {
				return nil, fmt.Errorf("failed to pull image %s and image not found locally: %w", imgName, err)
			}
			log.Detail("Image %s not pulled but exists locally", imgName)
		} else {
			defer reader.Close()
			_, _ = io.Copy(io.Discard, reader)
			log.Detail("Successfully pulled image %s", imgName)
		}
	}

	dns, err := parseDNSServers(backendSpecificParams.DNS)
	if err != nil {
		return nil, err
	}

	// Create instances
	log.Detail("Creating %d instances", input.Nodes)
	// create instances
	runResults := []client.ContainerCreateResult{}
	for i := lastNodeNo; i < lastNodeNo+input.Nodes; i++ {
		// Add node number tag
		nodeTags := make(map[string]string, len(labels))
		maps.Copy(nodeTags, labels)
		nodeTags[TAG_NODE_NO] = fmt.Sprintf("%d", i+1)
		name := input.Name
		if name == "" {
			name = fmt.Sprintf("%s-%s-%d", s.project, input.ClusterName, i+1)
		}
		nodeTags[TAG_NAME] = name
		// Create instance
		mounts := []mount.Mount{}
		for _, volume := range backendSpecificParams.Disks {
			vsplit := strings.Split(volume, ":")
			// Format: source:target or source:target:ro/rw
			if len(vsplit) < 2 || len(vsplit) > 3 {
				return nil, fmt.Errorf("invalid disk format: %s (expected source:target or source:target:ro)", volume)
			}
			// Determine mount type: use bind mount for absolute paths, volume for named volumes
			mountType := mount.TypeVolume
			if strings.HasPrefix(vsplit[0], "/") {
				mountType = mount.TypeBind
			}
			readOnly := false
			if len(vsplit) == 3 {
				switch vsplit[2] {
				case "ro":
					readOnly = true
				case "rw":
					readOnly = false
				default:
					return nil, fmt.Errorf("invalid disk format: %s (third part must be 'ro' or 'rw')", volume)
				}
			}
			mounts = append(mounts, mount.Mount{
				Type:     mountType,
				Source:   vsplit[0],
				Target:   vsplit[1],
				ReadOnly: readOnly,
			})
		}

		// get port bindings, and add them to used port list for the next looped run
		exposedPorts, portBindings, portList, err := s.getExposedPorts(backendSpecificParams.Firewalls)
		if err != nil {
			return nil, err
		}

		defer s.usedPorts.release(portList)

		rp := container.RestartPolicy{}
		if backendSpecificParams.RestartPolicy != "" {
			switch backendSpecificParams.RestartPolicy {
			case "Always":
				rp.Name = container.RestartPolicyAlways
			case "None":
				rp.Name = container.RestartPolicyDisabled
			case "OnFailure":
				rp.Name = container.RestartPolicyOnFailure
			case "UnlessStopped":
				rp.Name = container.RestartPolicyUnlessStopped
			default:
				return nil, fmt.Errorf("invalid restart policy: %s", backendSpecificParams.RestartPolicy)
			}
			rp.MaximumRetryCount = backendSpecificParams.MaxRestartRetries
		}
		// For custom images (not public/official aerolab images), preserve image defaults
		containerConfig := &container.Config{
			Hostname:        name,
			Domainname:      "aerolab.local",
			AttachStdin:     false,
			AttachStdout:    false,
			AttachStderr:    false,
			ExposedPorts:    exposedPorts,
			Tty:             true,
			OpenStdin:       false,
			StdinOnce:       false,
			Cmd:             backendSpecificParams.Cmd,
			ArgsEscaped:     false,
			Image:           imgName,
			Volumes:         nil,
			NetworkDisabled: false,
			Labels:          nodeTags,
			StopSignal:      "SIGTERM",
			StopTimeout:     backendSpecificParams.StopTimeout,
			Shell:           nil,
			// Always pass SSH key for aerolab to be able to connect
			Env: []string{"SSH_PUBLIC_KEY=" + string(publicKeyBytes)},
		}

		// Only set aerolab-specific defaults for official images, preserve defaults for custom images
		if backendSpecificParams.Image.Public {
			containerConfig.User = "root"
			containerConfig.WorkingDir = "/root"
		}

		runResult, err := cli.ContainerCreate(context.Background(), client.ContainerCreateOptions{
			Config: containerConfig,
			HostConfig: &container.HostConfig{
				NetworkMode:     container.NetworkMode(networkName),
				PortBindings:    portBindings,
				RestartPolicy:   rp,
				AutoRemove:      input.TerminateOnStop,
				ConsoleSize:     [2]uint{24, 80},
				CapAdd:          backendSpecificParams.CapAdd,
				CapDrop:         backendSpecificParams.CapDrop,
				DNS:             dns,
				DNSOptions:      backendSpecificParams.DNSOptions,
				DNSSearch:       backendSpecificParams.DNSSearch,
				Privileged:      backendSpecificParams.Privileged,
				PublishAllPorts: false, // crazy, docker will auto-map all exposed ports to random host ports
				SecurityOpt:     backendSpecificParams.SecurityOpt,
				Tmpfs:           backendSpecificParams.Tmpfs,
				ShmSize:         backendSpecificParams.ShmSize,
				Sysctls:         backendSpecificParams.Sysctls,
				Resources:       backendSpecificParams.Resources,
				Mounts:          mounts,
				MaskedPaths:     backendSpecificParams.MaskedPaths,
				ReadonlyPaths:   backendSpecificParams.ReadonlyPaths,
				Init:            nil, // do not install docker's init system
			},
			NetworkingConfig: &network.NetworkingConfig{
				EndpointsConfig: endpoints,
			},
			Platform: &v1.Platform{
				Architecture: backendSpecificParams.Image.Architecture.String(),
				OS:           "linux",
				OSVersion:    "",
				OSFeatures:   []string{},
				Variant:      "",
			},
			Name: name,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create instance %d: %v", i+1, err)
		}
		for _, w := range runResult.Warnings {
			if w == "" {
				continue
			}
			log.Warn("DOCKER: name=%s, warnings=%v", name, w)
		}
		if _, err := cli.ContainerStart(context.Background(), runResult.ID, client.ContainerStartOptions{}); err != nil {
			return nil, fmt.Errorf("failed to start instance %d: %v", i+1, err)
		}
		runResults = append(runResults, runResult)
	}

	// Starting a container is not the same as it being usable: published port
	// mappings and network settings only show up in the container list once the
	// container is actually running. Wait for that before listing, otherwise
	// every instance below is built from a summary with an empty port list and
	// a pre-running state.
	runIDs := make([]string, 0, len(runResults))
	for _, rr := range runResults {
		runIDs = append(runIDs, rr.ID)
	}
	if err := s.waitForContainersRunning(cli, runIDs, containerRunningBudget, log); err != nil {
		return nil, fmt.Errorf("%w%s", err, s.diagnoseStopped(cli, runIDs))
	}

	// get final instance details
	log.Detail("Getting final instance details")
	output = &backends.CreateInstanceOutput{
		Instances: backends.InstanceList{},
	}
	instances, err := s.GetInstances(s.volumes, s.networks, s.firewalls)
	if err != nil {
		return nil, fmt.Errorf("failed to get instances: %v", err)
	}
	for _, rr := range runResults {
		inst := instances.WithInstanceID(rr.ID)
		if inst.Count() != 1 {
			log.Warn("DOCKER: ID=%s, instance not found after creation", rr.ID)
			continue
		}
		output.Instances = append(output.Instances, inst.Describe()[0])
	}
	if len(output.Instances) != len(runResults) {
		return nil, fmt.Errorf("created %d instances but only %d were found in the container list", len(runResults), len(output.Instances))
	}

	if backendSpecificParams.SkipSshReadyCheck {
		return output, nil
	}

	// using ssh, wait for the instances to be ready
	log.Detail("Waiting for instances to be ssh-ready")
	if backendSpecificParams.Image.Username == "" {
		backendSpecificParams.Image.Username = "root"
	}
	var lastErrs error
	waitStart := time.Now()
	diagnosed := false
	for waitDur > 0 {
		now := time.Now()
		success := true
		// Re-read the container summaries while any SSH target is still
		// unresolved. The cached summary holds the published port list and
		// lifecycle state that InstancesExec uses to pick the target, so a
		// summary captured a moment too early would otherwise pin every
		// remaining attempt to a target that cannot possibly work. Once every
		// target resolves there is nothing left to learn from re-listing.
		if unresolvedSSHTargets(output.Instances) {
			if err := s.refreshStartedInstances(cli, runIDs, output.Instances); err != nil {
				log.Detail("Could not refresh container details: %s", err)
			}
		}
		out := output.Instances.Exec(&backends.ExecInput{
			Username:        backendSpecificParams.Image.Username,
			ParallelThreads: input.ParallelSSHThreads,
			ConnectTimeout:  5 * time.Second,
			ExecDetail: sshexec.ExecDetail{
				Command: []string{"ls", "/"},
			},
		})
		if len(out) != len(output.Instances) {
			success = false
		}
		lastErrs = nil
		failedIDs := []string{}
		for _, o := range out {
			if o.Output.Err != nil {
				success = false
				lastErrs = errors.Join(lastErrs, fmt.Errorf("%s: %w", o.Instance.Name, o.Output.Err))
				failedIDs = append(failedIDs, o.Instance.InstanceID)
				log.Detail("Waiting for instance %s to be ready: %s: %s", o.Instance.InstanceID, o.Output.Err, o.Output.Stdout)
			}
		}
		if success {
			break
		}
		// A container that is no longer running will never answer, because the
		// host port it published disappears with it - which on the client side
		// is indistinguishable from an sshd that has not come up yet. Stop as
		// soon as the daemon says the container is gone and report what it
		// did, instead of retrying a dead target until the budget expires.
		if stopped := stoppedContainers(cli, failedIDs); len(stopped) > 0 {
			return nil, fmt.Errorf("instances failed to initialize ssh because they stopped running:\n%s", describeContainers(cli, nil, stopped))
		}
		// The containers are alive but not answering. Say so once, part-way
		// through the budget, rather than leaving the user watching identical
		// connection errors scroll past until the whole budget is gone: a
		// running container whose port 22 is published and still refuses
		// connections is a host-side problem, and the evidence for that is the
		// container status below.
		if !diagnosed && time.Since(waitStart) > sshDiagnosticsAfter {
			diagnosed = true
			log.Warn("Instances have not answered ssh for %s; aerolab connects over 127.0.0.1 on the host port published for container port 22. Container status:\n%s", sshDiagnosticsAfter, describeContainers(cli, dockerExec(cli), runIDs))
		}
		waitDur -= time.Since(now)
		if waitDur > 0 {
			time.Sleep(1 * time.Second)
			waitDur -= 1 * time.Second
		}
	}

	if waitDur <= 0 {
		log.Detail("Instances failed to initialize ssh")
		if lastErrs != nil {
			return nil, fmt.Errorf("instances failed to initialize ssh: aerolab connects over 127.0.0.1 on the host port that docker/podman published for container port 22, so a refused connection means nothing is listening on that host port - either the container is not running, or the container engine's virtual machine is not forwarding published ports to the host (on macOS and Windows, restarting the docker/podman machine usually fixes that); last error(s): %w\n%s", lastErrs, describeContainers(cli, dockerExec(cli), runIDs))
		}
		return nil, fmt.Errorf("instances failed to initialize ssh\n%s", describeContainers(cli, dockerExec(cli), runIDs))
	}

	// return
	return output, nil
}

func (s *b) CleanupDNS() error {
	s.log.Detail("CleanupDNS: not implemented")
	return nil
}

func (s *b) InstancesUpdateHostsFile(instances backends.InstanceList, hostsEntries []string, parallelSSHThreads int) error {
	log := s.log.WithPrefix("InstancesUpdateHostsFile: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	// read update script template
	scriptBytes, err := scripts.ReadFile("scripts/update-hosts-file.sh")
	if err != nil {
		return fmt.Errorf("failed to read update-hosts-file.sh script: %v", err)
	}

	// format script with hosts entries
	script := fmt.Sprintf(string(scriptBytes), strings.Join(hostsEntries, "\n"))

	// upload script to the instances using ssh
	sshConfig, err := instances.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get sftp config: %v", err)
	}
	var retErr error
	wait := new(sync.WaitGroup)
	sem := make(chan struct{}, parallelSSHThreads)

	for _, config := range sshConfig {
		wait.Add(1)
		sem <- struct{}{}
		go func(config *sshexec.ClientConf) {
			defer wait.Done()
			defer func() { <-sem }()
			cli, err := sshexec.NewSftp(config)
			if err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("failed to create sftp client for host %s: %v", config.Host, err))
				return
			}
			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/tmp/update-hosts-file.sh",
				Source:      strings.NewReader(script),
				Permissions: 0755,
			})
			if err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("failed to write update-hosts-file.sh for host %s: %v", config.Host, err))
				return
			}
		}(config)
	}
	wait.Wait()
	if retErr != nil {
		return retErr
	}

	// execute script on all instances
	execInput := &backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:  []string{"bash", "/tmp/update-hosts-file.sh"},
			Terminal: true,
		},
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: parallelSSHThreads,
	}

	var errs error
	outputs := instances.Exec(execInput)
	for _, output := range outputs {
		if output.Output.Err != nil {
			log.Detail("ERROR: stdout: %s %s", output.Instance.ClusterName+"-"+strconv.Itoa(output.Instance.NodeNo), string(output.Output.Stdout))
			log.Detail("ERROR: stderr: %s %s", output.Instance.ClusterName+"-"+strconv.Itoa(output.Instance.NodeNo), string(output.Output.Stderr))
			log.Detail("ERROR: warn: %s %v", output.Instance.ClusterName+"-"+strconv.Itoa(output.Instance.NodeNo), output.Output.Warn)
			errs = errors.Join(errs, fmt.Errorf("failed to update hosts file on instance %s: %v", output.Instance.ClusterName+"-"+strconv.Itoa(output.Instance.NodeNo), output.Output.Err))
		}
	}
	return errs
}

// parseDNSServers converts the user-supplied DNS server list (kept as strings
// in the YAML/JSON params) into the netip.Addr slice HostConfig.DNS now wants.
func parseDNSServers(servers []string) ([]netip.Addr, error) {
	if len(servers) == 0 {
		return nil, nil
	}
	out := make([]netip.Addr, 0, len(servers))
	for _, server := range servers {
		addr, err := netip.ParseAddr(strings.TrimSpace(server))
		if err != nil {
			return nil, fmt.Errorf("invalid dns server %q: %w", server, err)
		}
		out = append(out, addr)
	}
	return out, nil
}

// tcpPort builds the network.Port key for a container TCP port given as a
// decimal string.
func tcpPort(port string) (network.Port, error) {
	p, err := network.ParsePort(port + "/tcp")
	if err != nil {
		return network.Port{}, fmt.Errorf("invalid container port %q: %w", port, err)
	}
	return p, nil
}

// anyIPv4 is 0.0.0.0, the default host address port bindings are published on.
var anyIPv4 = netip.AddrFrom4([4]byte{})

// parseHostIP converts the host side of a port mapping into the netip.Addr the
// API now expects, defaulting to 0.0.0.0 when unspecified.
func parseHostIP(ip string) (netip.Addr, error) {
	if ip == "" {
		return anyIPv4, nil
	}
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.Addr{}, fmt.Errorf("invalid host IP %q: %w", ip, err)
	}
	return addr, nil
}

func (s *b) getExposedPorts(firewalls []string) (network.PortSet, network.PortMap, []int, error) {
	portList := []int{}
	nextPort := s.usedPorts.getNextFree(2200)
	if nextPort == -1 {
		return nil, nil, nil, fmt.Errorf("no free ports available")
	}
	portList = append(portList, nextPort)
	sshPort := network.MustParsePort("22/tcp")
	exposedPorts := network.PortSet{
		sshPort: {},
	}
	portBindings := network.PortMap{
		sshPort: {
			{
				HostIP:   anyIPv4,
				HostPort: fmt.Sprintf("%d", nextPort),
			},
		},
	}
	for _, port := range firewalls {
		if strings.HasPrefix(port, "host=") || strings.HasPrefix(port, "container=") || strings.HasPrefix(port, "incr,") {
			// host=0.0.0.0:8080,container=80
			// host=8080,container=80
			split := strings.Split(port, ",")
			if len(split) != 2 {
				return nil, nil, nil, fmt.Errorf("invalid port format: %s", port)
			}
			hostip := ""
			hostport := ""
			container := ""
			incr := false
			for _, kv := range split {
				kv := strings.Split(kv, "=")
				if len(kv) != 2 {
					return nil, nil, nil, fmt.Errorf("invalid port format: %s", port)
				}
				switch kv[0] {
				case "host":
					ipPort := strings.Split(kv[1], ":")
					if len(ipPort) > 2 {
						return nil, nil, nil, fmt.Errorf("invalid port format: %s", port)
					}
					if len(ipPort) == 2 {
						hostip = ipPort[0]
						hostport = ipPort[1]
					} else {
						hostip = "0.0.0.0"
						hostport = ipPort[0]
					}
				case "container":
					container = kv[1]
				case "incr":
					incr = true
				default:
					return nil, nil, nil, fmt.Errorf("invalid port format: %s", port)
				}
			}
			if incr {
				hp, _ := strconv.Atoi(hostport)
				hp = s.usedPorts.getNextFree(hp)
				if hp == -1 {
					return nil, nil, nil, fmt.Errorf("no free ports available")
				}
				portList = append(portList, hp)
				hostport = fmt.Sprintf("%d", hp)
			} else {
				hp, _ := strconv.Atoi(hostport)
				if s.usedPorts.get(hp) {
					portList = append(portList, hp)
					hostport = fmt.Sprintf("%d", hp)
				} else {
					return nil, nil, nil, fmt.Errorf("port %d is already used", hp)
				}
			}
			cPort, err := tcpPort(container)
			if err != nil {
				return nil, nil, nil, err
			}
			hostAddr, err := parseHostIP(hostip)
			if err != nil {
				return nil, nil, nil, err
			}
			exposedPorts[cPort] = struct{}{}
			portBindings[cPort] = []network.PortBinding{
				{
					HostIP:   hostAddr,
					HostPort: hostport,
				},
			}
		} else if strings.Contains(port, ",") {
			// 0.0.0.0:8080,80
			incr := false
			if strings.HasPrefix(port, "+") {
				incr = true
				port = strings.TrimPrefix(port, "+")
			}
			split := strings.Split(port, ",")
			if len(split) != 2 {
				return nil, nil, nil, fmt.Errorf("invalid port format: %s", port)
			}
			ipPort := strings.Split(split[0], ":")
			if len(ipPort) != 2 {
				return nil, nil, nil, fmt.Errorf("invalid port format: %s", port)
			}
			if incr {
				hp, _ := strconv.Atoi(ipPort[1])
				hp = s.usedPorts.getNextFree(hp)
				if hp == -1 {
					return nil, nil, nil, fmt.Errorf("no free ports available")
				}
				portList = append(portList, hp)
				ipPort[1] = fmt.Sprintf("%d", hp)
			} else {
				hp, _ := strconv.Atoi(ipPort[1])
				if s.usedPorts.get(hp) {
					portList = append(portList, hp)
					ipPort[1] = fmt.Sprintf("%d", hp)
				} else {
					return nil, nil, nil, fmt.Errorf("port %d is already used", hp)
				}
			}
			cPort, err := tcpPort(split[1])
			if err != nil {
				return nil, nil, nil, err
			}
			hostAddr, err := parseHostIP(ipPort[0])
			if err != nil {
				return nil, nil, nil, err
			}
			exposedPorts[cPort] = struct{}{}
			portBindings[cPort] = []network.PortBinding{
				{
					HostIP:   hostAddr,
					HostPort: ipPort[1],
				},
			}
		} else {
			// 8080:80
			incr := false
			if strings.HasPrefix(port, "+") {
				incr = true
				port = strings.TrimPrefix(port, "+")
			}
			split := strings.Split(port, ":")
			if len(split) != 2 {
				return nil, nil, nil, fmt.Errorf("invalid port format: %s", port)
			}
			if incr {
				hp, _ := strconv.Atoi(split[0])
				hp = s.usedPorts.getNextFree(hp)
				if hp == -1 {
					return nil, nil, nil, fmt.Errorf("no free ports available")
				}
				portList = append(portList, hp)
				split[0] = fmt.Sprintf("%d", hp)
			} else {
				hp, _ := strconv.Atoi(split[0])
				if s.usedPorts.get(hp) {
					portList = append(portList, hp)
					split[0] = fmt.Sprintf("%d", hp)
				} else {
					return nil, nil, nil, fmt.Errorf("port %d is already used", hp)
				}
			}
			cPort, err := tcpPort(split[1])
			if err != nil {
				return nil, nil, nil, err
			}
			exposedPorts[cPort] = struct{}{}
			portBindings[cPort] = []network.PortBinding{
				{
					HostIP:   anyIPv4,
					HostPort: split[0],
				},
			}
		}
	}
	return exposedPorts, portBindings, portList, nil
}

// this should always return nil
func (s *b) ResolveNetworkPlacement(placement string) (vpc *backends.Network, subnet *backends.Subnet, zone string, err error) {
	return nil, nil, "", nil
}

// if using os.Stdin, you may want to provide io.NopCloser(os.Stdin) instead, to avoid closing stdin
func ExecWithCLI(
	ctx context.Context,
	cli *client.Client,
	containerID string,
	cmd []string,
	env []string,
	stdin io.ReadCloser,
	stdout io.Writer,
	stderr io.Writer,
	tty bool,
) (int, error) {
	// 1) create the exec
	createResp, err := cli.ExecCreate(ctx, containerID, client.ExecCreateOptions{
		Cmd:          cmd,
		Env:          env,
		TTY:          tty,
		AttachStdin:  stdin != nil,
		AttachStdout: stdout != nil,
		AttachStderr: stderr != nil && !tty, // TTY merges stderr into stdout
	})
	if err != nil {
		return -1, err
	}
	execID := createResp.ID

	// 2) attach & start via docker/cli
	opts := client.ExecAttachOptions{
		TTY: tty,
		// DetachKeys: "ctrl-p,ctrl-q", // set if you care
	}
	resp, err := cli.ExecAttach(ctx, execID, opts)
	if err != nil {
		return -1, err
	}
	// The connection is torn down here and nowhere else: the copy goroutine
	// below must not close it, or an in-flight read on the shared reader dies
	// with it.
	defer resp.Close()
	if tty {
		sshexec.AddRestoreRequest()
		defer sshexec.RestoreTerminal()
		if term.IsTerminal(os.Stdin.Fd()) {
			term.MakeRaw(os.Stdin.Fd()) //nolint:errcheck
		}
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}

	// Without a TTY the daemon frames stdout and stderr into one stream, so the
	// bytes have to be demultiplexed rather than copied through. The media type
	// reported on the hijacked connection is authoritative; fall back to what we
	// asked for if the daemon did not send one.
	rawStream := tty
	if mediaType, ok := resp.MediaType(); ok {
		rawStream = mediaType == types.MediaTypeRawStream
	}

	// A single reader: resp.Reader is a *bufio.Reader and is not safe for
	// concurrent use.
	copyDone := make(chan struct{})
	go func() {
		defer close(copyDone)
		if stdin != nil {
			// Matches the documented contract: the caller's stdin is closed once
			// the command has stopped producing output.
			defer stdin.Close() //nolint:errcheck
		}
		if rawStream {
			io.Copy(stdout, resp.Reader) //nolint:errcheck
		} else {
			stdcopy.StdCopy(stdout, stderr, resp.Reader) //nolint:errcheck
		}
	}()

	if stdin != nil {
		go func() {
			io.Copy(resp.Conn, stdin) //nolint:errcheck
			resp.CloseWrite()         //nolint:errcheck
		}()
	}

	// 3) wait for the output to drain, then collect the exit code. Waiting for
	// EOF before returning matters for more than completeness: callers read the
	// buffers they passed in as soon as this returns.
	select {
	case <-copyDone:
	case <-ctx.Done():
		return -1, ctx.Err()
	}

	// The daemon closes the stream when the exec process exits, so EOF normally
	// means it is done. A detach would also close the stream, so poll in that
	// case rather than reporting a bogus exit code.
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		inspect, err := cli.ExecInspect(ctx, execID, client.ExecInspectOptions{})
		if err != nil {
			return -1, err
		}
		if !inspect.Running {
			return inspect.ExitCode, nil
		}
		select {
		case <-ctx.Done():
			return -1, ctx.Err()
		case <-t.C:
		}
	}
}

// encodeAuthToBase64 encodes docker auth config to base64 for use with ImagePull
func encodeAuthToBase64(authConfig registry.AuthConfig) (string, error) {
	authJSON, err := json.Marshal(authConfig)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(authJSON), nil
}

// computeAccessURL computes the access URL for a client instance based on its type and port mappings.
// For Docker, it returns http://localhost:{hostPort} where hostPort is mapped to the client's service port.
//
// Parameters:
//   - clientType: the value of the aerolab.client.type tag (e.g., "vscode", "ams", "graph")
//   - ports: the Docker port mappings from the container
//
// Returns:
//   - string: the computed access URL, or empty string if not applicable
func computeAccessURL(clientType string, ports []container.PortSummary) string {
	if clientType == "" {
		return ""
	}

	// Map client types to their default container ports
	var containerPort uint16
	switch clientType {
	case "vscode":
		containerPort = 8080
	case "ams":
		containerPort = 3000 // Grafana
	case "graph":
		containerPort = 9090 // Prometheus metrics
	default:
		return ""
	}

	// Find the host port mapped to the container port
	for _, port := range ports {
		if port.PrivatePort == containerPort && port.PublicPort > 0 {
			return fmt.Sprintf("http://localhost:%d", port.PublicPort)
		}
	}

	return ""
}
