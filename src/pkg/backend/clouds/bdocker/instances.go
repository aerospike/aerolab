package bdocker

import (
	"archive/tar"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/parallelize"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/structtags"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/strslice"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/crypto/ssh"
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
	Cmd               strslice.StrSlice   `yaml:"cmd" json:"cmd"`
	StopTimeout       *int                `yaml:"stopTimeout" json:"stopTimeout"` // seconds
	CapAdd            strslice.StrSlice   `yaml:"capAdd" json:"capAdd"`
	CapDrop           strslice.StrSlice   `yaml:"capDrop" json:"capDrop"`
	DNS               strslice.StrSlice   `yaml:"dns" json:"dns"`
	DNSOptions        strslice.StrSlice   `yaml:"dnsOptions" json:"dnsOptions"`
	DNSSearch         strslice.StrSlice   `yaml:"dnsSearch" json:"dnsSearch"`
	Privileged        bool                `yaml:"privileged" json:"privileged"`
	SecurityOpt       strslice.StrSlice   `yaml:"securityOpt" json:"securityOpt"`
	Tmpfs             map[string]string   `yaml:"tmpfs" json:"tmpfs"`
	RestartPolicy     string              `yaml:"restartPolicy" json:"restartPolicy"` // Always,None,OnFailure,UnlessStopped
	MaxRestartRetries int                 `yaml:"maxRestartRetries" json:"maxRestartRetries"`
	ShmSize           int64               `yaml:"shmSize" json:"shmSize"`
	Sysctls           map[string]string   `yaml:"sysctls" json:"sysctls"` // format: key=value of sysctl commands, like net.ipv4.ip_forward=1
	Resources         container.Resources `yaml:"resources" json:"resources"`
	MaskedPaths       strslice.StrSlice   `yaml:"maskedPaths" json:"maskedPaths"`
	ReadonlyPaths     strslice.StrSlice   `yaml:"readonlyPaths" json:"readonlyPaths"`
}

type InstanceDetail struct {
	Docker container.Summary `json:"docker" yaml:"docker"`
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

			f := filters.NewArgs()
			if !s.listAllProjects {
				f.Add("label", TAG_AEROLAB_PROJECT+"="+s.project)
			}
			containers, err := cli.ContainerList(context.Background(), container.ListOptions{
				Size:    true,
				All:     true,
				Latest:  true,
				Filters: f,
			})
			if err != nil {
				errs = errors.Join(errs, err)
				return
			}
			for _, container := range containers {
				if container.Labels[TAG_AEROLAB_VERSION] == "" {
					continue
				}
				nodeNo, _ := strconv.Atoi(container.Labels[TAG_NODE_NO])
				var arch backends.Architecture
				arch.FromString(container.Labels[TAG_ARCHITECTURE])
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
				if net != nil {
					ip = net.IPAddress
				}
				istate := backends.LifeCycleStateRunning
				switch container.State {
				case "running":
					istate = backends.LifeCycleStateRunning
				case "exited":
					istate = backends.LifeCycleStateStopped
				case "dead":
					istate = backends.LifeCycleStateFail
				case "paused":
					istate = backends.LifeCycleStateUnknown
				case "restarting":
					istate = backends.LifeCycleStateStarting
				case "created":
					istate = backends.LifeCycleStateCreated
				}
				fw := []string{}
				for _, port := range container.Ports {
					// fw format: host={hostIP:hostPORT},container={containerPORT} ; example: host=0.0.0.0:8080,container=80
					fw = append(fw, fmt.Sprintf("host=%s:%d,container=%d", port.IP, port.PublicPort, port.PrivatePort))
				}
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
	log := s.log.WithPrefix("InstancesTerminate: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}

	removeSSHKey := false
	if s.instances.WithBackendType(backends.BackendTypeDocker).WithNotState(backends.LifeCycleStateTerminating, backends.LifeCycleStateTerminated).Count() == instances.Count() {
		removeSSHKey = true
	}

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)
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
				err := cli.ContainerRemove(context.Background(), id, container.RemoveOptions{
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

	// if no more instances exist for this project, delete the ssh key from amazon and locally from filepath.Join(s.sshKeysDir, s.project)
	if removeSSHKey && s.createInstanceCount.Get() == 0 {
		log.Detail("Remove SSH keys as no more instances exist for this project")
		os.Remove(filepath.Join(s.sshKeysDir, s.project))
		os.Remove(filepath.Join(s.sshKeysDir, s.project+".pub"))
		log.Detail("SSH keys removed")
	}
	return nil
}

func (s *b) InstancesStop(instances backends.InstanceList, force bool, waitDur time.Duration) error {
	log := s.log.WithPrefix("InstancesStop: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
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
				err := cli.ContainerStop(context.Background(), id, container.StopOptions{
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
					inspected, err := cli.ContainerInspect(context.Background(), id)
					if err != nil {
						return err
					}
					if !inspected.State.Running {
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
	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
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
				log.Detail("starting container %s", id)
				err := cli.ContainerStart(context.Background(), id, container.StartOptions{})
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
	return nil
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
		sshPort := 0
		for _, x := range i.BackendSpecific.(*InstanceDetail).Docker.Ports {
			if x.PrivatePort == 22 {
				sshPort = int(x.PublicPort)
				break
			}
		}
		clientConf := sshexec.ClientConf{
			Host:           "127.0.0.1",
			Port:           sshPort,
			Username:       e.Username,
			PrivateKey:     nKey,
			ConnectTimeout: e.ConnectTimeout,
		}
		execInput := &sshexec.ExecInput{
			ClientConf: clientConf,
			ExecDetail: e.ExecDetail,
		}
		execInput.ExecDetail.Env = append(execInput.ExecDetail.Env, &sshexec.Env{
			Key:   "AEROLAB_CLUSTER_NAME",
			Value: i.ClusterName,
		})
		execInput.ExecDetail.Env = append(execInput.ExecDetail.Env, &sshexec.Env{
			Key:   "AEROLAB_NODE_NO",
			Value: strconv.Itoa(i.NodeNo),
		})
		execInput.ExecDetail.Env = append(execInput.ExecDetail.Env, &sshexec.Env{
			Key:   "AEROLAB_PROJECT_NAME",
			Value: s.project,
		})
		execInput.ExecDetail.Env = append(execInput.ExecDetail.Env, &sshexec.Env{
			Key:   "AEROLAB_OWNER",
			Value: i.Owner,
		})
		o := sshexec.Exec(execInput)
		outl.Lock()
		out = append(out, &backends.ExecOutput{
			Output:   o,
			Instance: i,
		})
		outl.Unlock()
	})
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
		sshPort := 0
		for _, x := range i.BackendSpecific.(*InstanceDetail).Docker.Ports {
			if x.PrivatePort == 22 {
				sshPort = int(x.PublicPort)
				break
			}
		}
		clientConf := &sshexec.ClientConf{
			Host:           "127.0.0.1",
			Port:           sshPort,
			Username:       username,
			PrivateKey:     nKey,
			ConnectTimeout: 30 * time.Second,
		}
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
	log := s.log.WithPrefix("InstancesRemoveFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")
	if len(instances) == 0 {
		return nil
	}
	return errors.New("not implemented")
}

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

	// create docker tags for docker.CreateInstancesInput
	labels := map[string]string{
		TAG_OWNER:           input.Owner,
		TAG_CLUSTER_NAME:    input.ClusterName,
		TAG_DESCRIPTION:     input.Description,
		TAG_EXPIRES:         input.Expires.Format(time.RFC3339),
		TAG_AEROLAB_PROJECT: s.project,
		TAG_AEROLAB_VERSION: s.aerolabVersion,
		TAG_OS_NAME:         backendSpecificParams.Image.OSName,
		TAG_OS_VERSION:      backendSpecificParams.Image.OSVersion,
		TAG_CLUSTER_UUID:    clusterUUID,
	}
	for k, v := range input.Tags {
		labels[k] = v
	}

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
	sshKeyPath := filepath.Join(s.sshKeysDir, s.project)

	// if key does not exist in docker, create it
	var publicKeyBytes []byte
	if _, err := os.Stat(sshKeyPath); os.IsNotExist(err) {
		log.Detail("SSH key %s does not exist, creating it", sshKeyPath)
		// generate new SSH key pair
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("failed to generate private key: %v", err)
		}

		// encode public key
		publicKey, err := ssh.NewPublicKey(&privateKey.PublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to create public key: %v", err)
		}
		publicKeyBytes = ssh.MarshalAuthorizedKey(publicKey)

		// save private key to file
		privateKeyBytes := pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
		})

		if _, err := os.Stat(s.sshKeysDir); os.IsNotExist(err) {
			err = os.MkdirAll(s.sshKeysDir, 0700)
			if err != nil {
				return nil, fmt.Errorf("failed to create ssh keys directory: %v", err)
			}
		}

		err = os.WriteFile(sshKeyPath, privateKeyBytes, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to save private key: %v", err)
		}

		err = os.WriteFile(sshKeyPath+".pub", publicKeyBytes, 0600)
		if err != nil {
			return nil, fmt.Errorf("failed to save public key: %v", err)
		}
	} else {
		publicKeyBytes, err = os.ReadFile(sshKeyPath + ".pub")
		if err != nil {
			return nil, fmt.Errorf("failed to read public key: %v", err)
		}
	}
	publicKeyBytes = bytes.Trim(publicKeyBytes, "\n\r\t ")

	// if image is public, check if we have a custom build already; if not: make one
	// we need to track who is building the image, so that if another CreateInstances is already building this particular image, we should just wait for it to finish
	imgName := backendSpecificParams.Image.Name
	if backendSpecificParams.Image.Public {
		if backendSpecificParams.Image.BackendSpecific.(*ImageDetail).Docker == nil {
			s.builderMutex.Lock()
			if _, ok := s.builders[backendSpecificParams.Image.ZoneName]; !ok {
				s.builders[backendSpecificParams.Image.ZoneName] = make(map[string]*dockerBuilder)
			}
			if builder, ok := s.builders[backendSpecificParams.Image.ZoneName][backendSpecificParams.Image.Name]; ok {
				builder.wg.Wait()
				if builder.docker != nil {
					reassign := backendSpecificParams.Image.BackendSpecific.(*ImageDetail)
					reassign.Docker = builder.docker
					backendSpecificParams.Image.BackendSpecific = reassign
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
					df = []byte(fmt.Sprintf(string(df), backendSpecificParams.Image.Name))
					buf := new(bytes.Buffer)
					tw := tar.NewWriter(buf)
					tw.WriteHeader(&tar.Header{
						Name: "Dockerfile",
						Mode: 0644,
						Size: int64(len(df)),
					})
					tw.Write(df)
					tw.WriteHeader(&tar.Header{
						Name: "userdata.sh",
						Mode: 0755,
						Size: int64(len(ud)),
					})
					tw.Write(ud)
					tw.Flush()
					tw.Close()
					pf := "linux/amd64"
					if backendSpecificParams.Image.Architecture == backends.ArchitectureARM64 {
						pf = "linux/arm64"
					}
					newNameTag := backendSpecificParams.Image.Architecture.String() + "-" + backendSpecificParams.Image.OSName + "-" + backendSpecificParams.Image.OSVersion
					if s.isPodman[backendSpecificParams.Image.ZoneName] {
						newNameTag = "localhost/" + newNameTag
					}
					builder, err := cli.ImageBuild(context.Background(), buf, types.ImageBuildOptions{
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
						Platform:       pf,
						Outputs:        []types.ImageBuildOutput{},
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

					reassign := backendSpecificParams.Image.BackendSpecific.(*ImageDetail)
					reassign.Docker = &image.Summary{
						Created:  time.Now().Unix(),
						ID:       newNameTag,
						Labels:   imgLabels,
						ParentID: backendSpecificParams.Image.Name,
						RepoTags: []string{newNameTag},
					}
					backendSpecificParams.Image.BackendSpecific = reassign
					s.builders[backendSpecificParams.Image.ZoneName][backendSpecificParams.Image.Name].docker = reassign.Docker
					return nil
				}()
				if err != nil {
					return nil, err
				}
			}
		}
		if len(backendSpecificParams.Image.BackendSpecific.(*ImageDetail).Docker.RepoTags) > 0 {
			imgName = backendSpecificParams.Image.BackendSpecific.(*ImageDetail).Docker.RepoTags[0]
		} else {
			imgName = backendSpecificParams.Image.BackendSpecific.(*ImageDetail).Docker.ID
		}
	}

	// Create instances
	log.Detail("Creating %d instances", input.Nodes)
	// create instances
	runResults := []container.CreateResponse{}
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
			if len(vsplit) != 2 {
				return nil, fmt.Errorf("invalid disk format: %s", volume)
			}
			mounts = append(mounts, mount.Mount{
				Type:   mount.TypeVolume,
				Source: vsplit[0],
				Target: vsplit[1],
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
		runResult, err := cli.ContainerCreate(context.Background(), &container.Config{
			Hostname:     name,
			Domainname:   "aerolab.local",
			User:         "root",
			AttachStdin:  false,
			AttachStdout: false,
			AttachStderr: false,
			ExposedPorts: exposedPorts,
			Tty:          true,
			OpenStdin:    false, // systemd should not need stdin
			StdinOnce:    false,
			Env: []string{
				"SSH_PUBLIC_KEY=" + string(publicKeyBytes),
			},
			Cmd:             backendSpecificParams.Cmd,
			ArgsEscaped:     false,
			Image:           imgName,
			Volumes:         nil, // anonymous volumes, do not use
			WorkingDir:      "/root",
			Entrypoint:      nil, // can be used to override entrypoint
			NetworkDisabled: false,
			Labels:          nodeTags,
			StopSignal:      "SIGTERM",
			StopTimeout:     backendSpecificParams.StopTimeout,
			Shell:           nil, // default is fine
		}, &container.HostConfig{
			NetworkMode:     container.NetworkMode(networkName),
			PortBindings:    portBindings,
			RestartPolicy:   rp,
			AutoRemove:      input.TerminateOnStop,
			ConsoleSize:     [2]uint{24, 80},
			CapAdd:          backendSpecificParams.CapAdd,
			CapDrop:         backendSpecificParams.CapDrop,
			DNS:             backendSpecificParams.DNS,
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
		}, &network.NetworkingConfig{
			EndpointsConfig: endpoints,
		}, &v1.Platform{
			Architecture: backendSpecificParams.Image.Architecture.String(),
			OS:           "linux",
			OSVersion:    "",
			OSFeatures:   []string{},
			Variant:      "",
		}, name)
		if err != nil {
			return nil, fmt.Errorf("failed to create instance %d: %v", i+1, err)
		}
		if runResult.Warnings != nil {
			log.Warn("DOCKER: name=%s, warnings=%v", name, runResult.Warnings)
		}
		if err := cli.ContainerStart(context.Background(), runResult.ID, container.StartOptions{}); err != nil {
			return nil, fmt.Errorf("failed to start instance %d: %v", i+1, err)
		}
		runResults = append(runResults, runResult)
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

	// using ssh, wait for the instances to be ready
	log.Detail("Waiting for instances to be ssh-ready")
	for waitDur > 0 {
		now := time.Now()
		success := true
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
		for _, o := range out {
			if o.Output.Err != nil {
				success = false
				log.Detail("Waiting for instance %s to be ready: %s: %s", o.Instance.InstanceID, o.Output.Err, o.Output.Stdout)
			}
		}
		if success {
			break
		}
		waitDur -= time.Since(now)
		if waitDur > 0 {
			time.Sleep(1 * time.Second)
		}
	}

	if waitDur <= 0 {
		log.Detail("Instances failed to initialize ssh")
		return nil, fmt.Errorf("instances failed to initialize ssh")
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
		config := config
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

func (s *b) getExposedPorts(firewalls []string) (nat.PortSet, nat.PortMap, []int, error) {
	portList := []int{}
	nextPort := s.usedPorts.getNextFree(2200)
	if nextPort == -1 {
		return nil, nil, nil, fmt.Errorf("no free ports available")
	}
	portList = append(portList, nextPort)
	exposedPorts := nat.PortSet{
		"22/tcp": {},
	}
	portBindings := nat.PortMap{
		"22/tcp": {
			{
				HostIP:   "0.0.0.0",
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
			exposedPorts[nat.Port(container+"/tcp")] = struct{}{}
			portBindings[nat.Port(container+"/tcp")] = []nat.PortBinding{
				{
					HostIP:   hostip,
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
			exposedPorts[nat.Port(split[1]+"/tcp")] = struct{}{}
			portBindings[nat.Port(split[1]+"/tcp")] = []nat.PortBinding{
				{
					HostIP:   ipPort[0],
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
			exposedPorts[nat.Port(split[1]+"/tcp")] = struct{}{}
			portBindings[nat.Port(split[1]+"/tcp")] = []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: split[0],
				},
			}
		}
	}
	return exposedPorts, portBindings, portList, nil
}
