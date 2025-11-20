package bvagrant

import (
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
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/aerospike/aerolab/pkg/utils/structtags"
	"github.com/google/uuid"
	"github.com/lithammer/shortuuid"
	"golang.org/x/crypto/ssh"
)

// CreateInstanceParams contains Vagrant-specific parameters for instance creation.
type CreateInstanceParams struct {
	// Box name (e.g. "ubuntu/jammy64", "generic/ubuntu2204")
	Box string `yaml:"box" json:"box" required:"true"`
	// Box version (optional, leave empty for latest)
	BoxVersion string `yaml:"boxVersion" json:"boxVersion"`
	// CPU count
	CPUs int `yaml:"cpus" json:"cpus" required:"true"`
	// Memory in MB
	Memory int `yaml:"memory" json:"memory" required:"true"`
	// Disk size in GB (only for providers that support it)
	DiskSize int `yaml:"diskSize" json:"diskSize"`
	// Network type: "private_network" or "public_network"
	NetworkType string `yaml:"networkType" json:"networkType"`
	// Network IP (for private_network with static IP)
	NetworkIP string `yaml:"networkIP" json:"networkIP"`
	// Synced folders map: {hostPath}:{guestPath}
	SyncedFolders map[string]string `yaml:"syncedFolders" json:"syncedFolders"`
	// Port forwards: {guest}:{host}
	PortForwards map[int]int `yaml:"portForwards" json:"portForwards"`
	// Skip SSH ready check
	SkipSshReadyCheck bool `yaml:"skipSshReadyCheck" json:"skipSshReadyCheck"`
}

// InstanceDetail contains Vagrant-specific instance information.
type InstanceDetail struct {
	VagrantID  string            `json:"vagrantId" yaml:"vagrantId"`
	VagrantDir string            `json:"vagrantDir" yaml:"vagrantDir"`
	BoxName    string            `json:"boxName" yaml:"boxName"`
	BoxVersion string            `json:"boxVersion" yaml:"boxVersion"`
	Provider   string            `json:"provider" yaml:"provider"`
	SSHInfo    *VagrantSSHInfo   `json:"sshInfo" yaml:"sshInfo"`
	Metadata   map[string]string `json:"metadata" yaml:"metadata"`
}

// VagrantSSHInfo contains SSH connection information for a Vagrant VM.
type VagrantSSHInfo struct {
	Host         string `json:"host" yaml:"host"`
	Port         int    `json:"port" yaml:"port"`
	User         string `json:"user" yaml:"user"`
	PrivateKey   string `json:"privateKey" yaml:"privateKey"`
	ForwardAgent bool   `json:"forwardAgent" yaml:"forwardAgent"`
}

// GetInstances retrieves all Vagrant VM instances.
//
// Parameters:
//   - volumes: volume list for attachment information
//   - networkList: network list for network information
//   - firewallList: firewall list for firewall information
//
// Returns:
//   - backends.InstanceList: list of instances
//   - error: nil on success, or an error describing what failed
func (s *b) GetInstances(volumes backends.VolumeList, networkList backends.NetworkList, firewallList backends.FirewallList) (backends.InstanceList, error) {
	log := s.log.WithPrefix("GetInstances: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	var instances backends.InstanceList
	instancesLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	zones, _ := s.ListEnabledZones()

	// If no zones are configured, use the default workDir
	if len(zones) == 0 {
		zones = []string{"default"}
	}

	wg.Add(len(zones))
	var errs error
	errsLock := new(sync.Mutex)

	for _, zone := range zones {
		go func(zone string) {
			defer wg.Done()
			log.Detail("zone=%s start", zone)
			defer log.Detail("zone=%s end", zone)

			workDir, err := s.getVagrantWorkDir(zone)
			if err != nil {
				errsLock.Lock()
				errs = errors.Join(errs, err)
				errsLock.Unlock()
				return
			}

			// Find all vagrant directories in the work directory
			entries, err := os.ReadDir(workDir)
			if err != nil {
				if os.IsNotExist(err) {
					// workDir doesn't exist yet, no instances
					return
				}
				errsLock.Lock()
				errs = errors.Join(errs, fmt.Errorf("failed to read vagrant work directory %s: %w", workDir, err))
				errsLock.Unlock()
				return
			}

			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}

				vmDir := filepath.Join(workDir, entry.Name())
				// Check if this is a Vagrant directory
				metadataFile := filepath.Join(vmDir, "metadata.json")
				if _, err := os.Stat(metadataFile); os.IsNotExist(err) {
					continue
				}

				// Read metadata
				metadataBytes, err := os.ReadFile(metadataFile)
				if err != nil {
					log.Warn("Failed to read metadata for %s: %v", vmDir, err)
					continue
				}

				var metadata map[string]string
				if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
					log.Warn("Failed to parse metadata for %s: %v", vmDir, err)
					continue
				}

				// Skip if not from our project (unless listAllProjects is true)
				if !s.listAllProjects && metadata[TAG_AEROLAB_PROJECT] != s.project {
					continue
				}

				// Skip if no aerolab version tag (not an aerolab-managed VM)
				if metadata[TAG_AEROLAB_VERSION] == "" {
					continue
				}

				// Get Vagrant status
				state, err := s.getVagrantStatus(vmDir)
				if err != nil {
					log.Warn("Failed to get status for %s: %v", vmDir, err)
					continue
				}

				// Parse metadata
				nodeNo, _ := strconv.Atoi(metadata[TAG_NODE_NO])
				var arch backends.Architecture
				arch.FromString(metadata[TAG_ARCHITECTURE])
				createTime, _ := time.Parse(time.RFC3339, metadata["createTime"])
				expires := time.Time{}
				if val, ok := metadata[TAG_EXPIRES]; ok {
					expires, _ = time.Parse(time.RFC3339, val)
				}

				// Get SSH info
				sshInfo, err := s.getVagrantSSHConfig(vmDir)
				if err != nil {
					log.Warn("Failed to get SSH config for %s: %v", vmDir, err)
					sshInfo = nil
				}

				// Convert Vagrant state to lifecycle state
				lifecycleState := s.vagrantStateToLifecycleState(state)

				// Get IP address if VM is running
				// For Vagrant, we need to distinguish between:
				// - The forwarded SSH host:port (127.0.0.1:2222) for SSH/SFTP connections
				// - The actual internal IP of the VM for cluster listing
				ip := ""
				if sshInfo != nil && state == "running" {
					// Get the actual internal IP from the VM
					ip = s.getVagrantInternalIP(vmDir)
					if ip == "" {
						// Fallback to SSH host if we can't determine internal IP
						ip = sshInfo.Host
					}
				}

				instancesLock.Lock()
				instances = append(instances, &backends.Instance{
					ClusterName: metadata[TAG_CLUSTER_NAME],
					ClusterUUID: metadata[TAG_CLUSTER_UUID],
					NodeNo:      nodeNo,
					IP: backends.IP{
						Private: ip,
					},
					ImageID:      metadata["boxName"],
					SubnetID:     "",
					NetworkID:    "",
					Architecture: arch,
					OperatingSystem: backends.OS{
						Name:    metadata[TAG_OS_NAME],
						Version: metadata[TAG_OS_VERSION],
					},
					Firewalls:        []string{},
					InstanceID:       entry.Name(),
					BackendType:      backends.BackendTypeVagrant,
					InstanceType:     fmt.Sprintf("%scpu-%smb", metadata["cpus"], metadata["memory"]),
					SpotInstance:     false,
					Name:             metadata[TAG_NAME],
					ZoneName:         zone,
					ZoneID:           zone,
					CreationTime:     createTime,
					EstimatedCostUSD: backends.Cost{},
					AttachedVolumes:  nil,
					Owner:            metadata[TAG_OWNER],
					InstanceState:    lifecycleState,
					Tags:             metadata,
					Expires:          expires,
					Description:      metadata[TAG_DESCRIPTION],
					CustomDNS:        nil,
					BackendSpecific: &InstanceDetail{
						VagrantID:  entry.Name(),
						VagrantDir: vmDir,
						BoxName:    metadata["boxName"],
						BoxVersion: metadata["boxVersion"],
						Provider:   s.getVagrantProvider(),
						SSHInfo:    sshInfo,
						Metadata:   metadata,
					},
				})
				instancesLock.Unlock()
			}
		}(zone)
	}

	wg.Wait()

	if errs == nil {
		s.instances = instances
	}

	return instances, errs
}

// vagrantStateToLifecycleState converts Vagrant state to backends.LifeCycleState.
func (s *b) vagrantStateToLifecycleState(state string) backends.LifeCycleState {
	switch state {
	case "running":
		return backends.LifeCycleStateRunning
	case "poweroff", "saved":
		return backends.LifeCycleStateStopped
	case "aborted":
		return backends.LifeCycleStateFail
	case "not_created", "not created":
		return backends.LifeCycleStateTerminated
	case "preparing":
		return backends.LifeCycleStateCreating
	default:
		return backends.LifeCycleStateUnknown
	}
}

// getVagrantStatus gets the status of a Vagrant VM.
func (s *b) getVagrantStatus(vmDir string) (string, error) {
	cmd := exec.Command("vagrant", "status", "--machine-readable")
	cmd.Dir = vmDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("vagrant status failed: %w: %s", err, string(output))
	}

	// Parse machine-readable output
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		parts := strings.Split(line, ",")
		if len(parts) >= 4 && parts[2] == "state" {
			return parts[3], nil
		}
	}

	return "unknown", nil
}

// getVagrantInternalIP gets the internal IP address of a Vagrant VM.
// This returns the actual IP address of the VM, not the forwarded 127.0.0.1.
func (s *b) getVagrantInternalIP(vmDir string) string {
	// Try to get the IP using vagrant ssh
	cmd := exec.Command("vagrant", "ssh", "-c", "hostname -I | awk '{print $1}'")
	cmd.Dir = vmDir
	output, err := cmd.Output()
	if err != nil {
		// If that fails, try another method
		cmd = exec.Command("vagrant", "ssh", "-c", "ip -4 addr show | grep -oP '(?<=inet\\s)\\d+(\\.\\d+){3}' | grep -v '127.0.0.1' | head -1")
		cmd.Dir = vmDir
		output, err = cmd.Output()
		if err != nil {
			return ""
		}
	}

	ip := strings.TrimSpace(string(output))
	// Validate it's a proper IP
	if ip != "" && ip != "127.0.0.1" {
		return ip
	}
	return ""
}

// getVagrantSSHConfig gets SSH configuration for a Vagrant VM.
func (s *b) getVagrantSSHConfig(vmDir string) (*VagrantSSHInfo, error) {
	cmd := exec.Command("vagrant", "ssh-config")
	cmd.Dir = vmDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("vagrant ssh-config failed: %w: %s", err, string(output))
	}

	info := &VagrantSSHInfo{}
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		switch parts[0] {
		case "HostName":
			info.Host = parts[1]
		case "Port":
			info.Port, _ = strconv.Atoi(parts[1])
		case "User":
			info.User = parts[1]
		case "IdentityFile":
			info.PrivateKey = strings.Trim(parts[1], "\"")
		case "ForwardAgent":
			info.ForwardAgent = parts[1] == "yes"
		}
	}

	return info, nil
}

// InstancesAddTags adds tags to Vagrant VM instances.
func (s *b) InstancesAddTags(instances backends.InstanceList, tags map[string]string) error {
	log := s.log.WithPrefix("InstancesAddTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if len(instances) == 0 {
		return nil
	}

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)

	var errs error
	for _, instance := range instances {
		detail, ok := instance.BackendSpecific.(*InstanceDetail)
		if !ok {
			continue
		}

		// Read current metadata
		metadataFile := filepath.Join(detail.VagrantDir, "metadata.json")
		metadataBytes, err := os.ReadFile(metadataFile)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to read metadata for %s: %w", instance.Name, err))
			continue
		}

		var metadata map[string]string
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to parse metadata for %s: %w", instance.Name, err))
			continue
		}

		// Add new tags
		for k, v := range tags {
			metadata[k] = v
		}

		// Write updated metadata
		updatedBytes, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to marshal metadata for %s: %w", instance.Name, err))
			continue
		}

		if err := os.WriteFile(metadataFile, updatedBytes, 0644); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to write metadata for %s: %w", instance.Name, err))
			continue
		}
	}

	return errs
}

// InstancesRemoveTags removes tags from Vagrant VM instances.
func (s *b) InstancesRemoveTags(instances backends.InstanceList, tagKeys []string) error {
	log := s.log.WithPrefix("InstancesRemoveTags: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if len(instances) == 0 {
		return nil
	}

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)

	var errs error
	for _, instance := range instances {
		detail, ok := instance.BackendSpecific.(*InstanceDetail)
		if !ok {
			continue
		}

		// Read current metadata
		metadataFile := filepath.Join(detail.VagrantDir, "metadata.json")
		metadataBytes, err := os.ReadFile(metadataFile)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to read metadata for %s: %w", instance.Name, err))
			continue
		}

		var metadata map[string]string
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to parse metadata for %s: %w", instance.Name, err))
			continue
		}

		// Remove tags
		for _, k := range tagKeys {
			delete(metadata, k)
		}

		// Write updated metadata
		updatedBytes, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to marshal metadata for %s: %w", instance.Name, err))
			continue
		}

		if err := os.WriteFile(metadataFile, updatedBytes, 0644); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to write metadata for %s: %w", instance.Name, err))
			continue
		}
	}

	return errs
}

// InstancesTerminate terminates (destroys) Vagrant VM instances.
func (s *b) InstancesTerminate(instances backends.InstanceList, waitDur time.Duration) error {
	log := s.log.WithPrefix("InstancesTerminate: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if len(instances) == 0 {
		return nil
	}

	removeSSHKey := false
	if s.instances.WithBackendType(backends.BackendTypeVagrant).WithNotState(backends.LifeCycleStateTerminating, backends.LifeCycleStateTerminated).Count() == instances.Count() {
		removeSSHKey = true
	}

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)

	var errs error
	errsLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)

	for _, instance := range instances {
		wg.Add(1)
		go func(instance *backends.Instance) {
			defer wg.Done()

			detail, ok := instance.BackendSpecific.(*InstanceDetail)
			if !ok {
				errsLock.Lock()
				errs = errors.Join(errs, fmt.Errorf("invalid instance detail for %s", instance.Name))
				errsLock.Unlock()
				return
			}

			log.Detail("Destroying VM %s in %s", instance.Name, detail.VagrantDir)

			cmd := exec.Command("vagrant", "destroy", "-f")
			cmd.Dir = detail.VagrantDir
			output, err := cmd.CombinedOutput()
			if err != nil {
				errsLock.Lock()
				errs = errors.Join(errs, fmt.Errorf("failed to destroy %s: %w: %s", instance.Name, err, string(output)))
				errsLock.Unlock()
				return
			}

			// Remove the vagrant directory
			if err := os.RemoveAll(detail.VagrantDir); err != nil {
				log.Warn("Failed to remove vagrant directory %s: %v", detail.VagrantDir, err)
			}

			// Clear from cache
			s.vagrantCache.delete(instance.InstanceID)
		}(instance)
	}

	wg.Wait()

	// if no more instances exist for this project, delete the ssh key
	if removeSSHKey && s.createInstanceCount.Get() == 0 {
		log.Detail("Remove SSH keys as no more instances exist for this project")
		os.Remove(filepath.Join(s.sshKeysDir, s.project))
		os.Remove(filepath.Join(s.sshKeysDir, s.project+".pub"))
		log.Detail("SSH keys removed")
	}

	return errs
}

// InstancesStop stops Vagrant VM instances.
func (s *b) InstancesStop(instances backends.InstanceList, force bool, waitDur time.Duration) error {
	log := s.log.WithPrefix("InstancesStop: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if len(instances) == 0 {
		return nil
	}

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)

	var errs error
	errsLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)

	for _, instance := range instances {
		wg.Add(1)
		go func(instance *backends.Instance) {
			defer wg.Done()

			detail, ok := instance.BackendSpecific.(*InstanceDetail)
			if !ok {
				errsLock.Lock()
				errs = errors.Join(errs, fmt.Errorf("invalid instance detail for %s", instance.Name))
				errsLock.Unlock()
				return
			}

			log.Detail("Stopping VM %s in %s", instance.Name, detail.VagrantDir)

			cmd := exec.Command("vagrant", "halt")
			if force {
				cmd.Args = append(cmd.Args, "-f")
			}
			cmd.Dir = detail.VagrantDir
			output, err := cmd.CombinedOutput()
			if err != nil {
				errsLock.Lock()
				errs = errors.Join(errs, fmt.Errorf("failed to stop %s: %w: %s", instance.Name, err, string(output)))
				errsLock.Unlock()
				return
			}

			// Clear from cache
			s.vagrantCache.delete(instance.InstanceID)
		}(instance)
	}

	wg.Wait()

	return errs
}

// InstancesStart starts Vagrant VM instances.
func (s *b) InstancesStart(instances backends.InstanceList, waitDur time.Duration) error {
	log := s.log.WithPrefix("InstancesStart: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if len(instances) == 0 {
		return nil
	}

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)

	var errs error
	errsLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)

	for _, instance := range instances {
		wg.Add(1)
		go func(instance *backends.Instance) {
			defer wg.Done()

			detail, ok := instance.BackendSpecific.(*InstanceDetail)
			if !ok {
				errsLock.Lock()
				errs = errors.Join(errs, fmt.Errorf("invalid instance detail for %s", instance.Name))
				errsLock.Unlock()
				return
			}

			log.Detail("Starting VM %s in %s", instance.Name, detail.VagrantDir)

			cmd := exec.Command("vagrant", "up")
			cmd.Dir = detail.VagrantDir
			output, err := cmd.CombinedOutput()
			if err != nil {
				errsLock.Lock()
				errs = errors.Join(errs, fmt.Errorf("failed to start %s: %w: %s", instance.Name, err, string(output)))
				errsLock.Unlock()
				return
			}

			// Clear from cache
			s.vagrantCache.delete(instance.InstanceID)
		}(instance)
	}

	wg.Wait()

	return errs
}

// InstancesExec executes commands on Vagrant VM instances via `vagrant ssh`.
func (s *b) InstancesExec(instances backends.InstanceList, e *backends.ExecInput) []*backends.ExecOutput {
	log := s.log.WithPrefix("InstancesExec: job=" + shortuuid.New() + " ")
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

		detail, ok := i.BackendSpecific.(*InstanceDetail)
		if !ok {
			outl.Lock()
			out = append(out, &backends.ExecOutput{
				Output: &sshexec.ExecOutput{
					Err: errors.New("no instance detail available"),
				},
				Instance: i,
			})
			outl.Unlock()
			return
		}

		// For interactive terminal mode (Terminal=true, no command or command is shell),
		// use vagrant ssh directly without -c
		if e.ExecDetail.Terminal && (len(e.ExecDetail.Command) == 0 ||
			(len(e.ExecDetail.Command) == 1 && (e.ExecDetail.Command[0] == "bash" || e.ExecDetail.Command[0] == "sh"))) {
			// Interactive shell - use vagrant ssh directly
			cmd := exec.Command("vagrant", "ssh")
			cmd.Dir = detail.VagrantDir
			cmd.Stdin = e.ExecDetail.Stdin
			cmd.Stdout = e.ExecDetail.Stdout
			cmd.Stderr = e.ExecDetail.Stderr

			err := cmd.Run()

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

		// Non-interactive: Use vagrant ssh -c to execute commands
		// Build the command string with environment variables and the actual command
		var cmdBuilder strings.Builder

		// Add environment variables
		envVars := []*sshexec.Env{
			{Key: "AEROLAB_CLUSTER_NAME", Value: i.ClusterName},
			{Key: "AEROLAB_NODE_NO", Value: strconv.Itoa(i.NodeNo)},
			{Key: "AEROLAB_PROJECT_NAME", Value: s.project},
			{Key: "AEROLAB_OWNER", Value: i.Owner},
		}
		envVars = append(envVars, e.ExecDetail.Env...)

		for _, env := range envVars {
			cmdBuilder.WriteString(fmt.Sprintf("export %s=%s; ", env.Key, shellEscape(env.Value)))
		}

		// Add the actual command
		for idx, arg := range e.ExecDetail.Command {
			if idx > 0 {
				cmdBuilder.WriteString(" ")
			}
			cmdBuilder.WriteString(shellEscape(arg))
		}

		// If username is specified and not vagrant/root, use sudo
		cmdString := cmdBuilder.String()
		if e.Username != "" && e.Username != "vagrant" && e.Username != detail.SSHInfo.User {
			// For root or other users, use sudo
			if e.Username == "root" {
				cmdString = "sudo -i bash -c " + shellEscape(cmdString)
			} else {
				cmdString = fmt.Sprintf("sudo -u %s bash -c %s", shellEscape(e.Username), shellEscape(cmdString))
			}
		}

		// Execute using vagrant ssh
		cmd := exec.Command("vagrant", "ssh", "-c", cmdString)
		cmd.Dir = detail.VagrantDir

		// Set up stdin, stdout, stderr
		if e.ExecDetail.Stdin != nil {
			cmd.Stdin = e.ExecDetail.Stdin
		}
		if e.ExecDetail.Stdout != nil {
			cmd.Stdout = e.ExecDetail.Stdout
		}
		if e.ExecDetail.Stderr != nil {
			cmd.Stderr = e.ExecDetail.Stderr
		}

		// Capture output if no stdout/stderr specified
		var stdoutBuf, stderrBuf bytes.Buffer
		if e.ExecDetail.Stdout == nil {
			cmd.Stdout = &stdoutBuf
		}
		if e.ExecDetail.Stderr == nil {
			cmd.Stderr = &stderrBuf
		}

		// Set timeout context if specified
		var err error
		if e.ExecDetail.SessionTimeout > 0 {
			timeoutCtx, cancel := context.WithTimeout(context.Background(), e.ExecDetail.SessionTimeout)
			defer cancel()

			cmdWithTimeout := exec.CommandContext(timeoutCtx, "vagrant", "ssh", "-c", cmdString)
			cmdWithTimeout.Dir = detail.VagrantDir

			if e.ExecDetail.Stdin != nil {
				cmdWithTimeout.Stdin = e.ExecDetail.Stdin
			}
			if e.ExecDetail.Stdout != nil {
				cmdWithTimeout.Stdout = e.ExecDetail.Stdout
			} else {
				cmdWithTimeout.Stdout = &stdoutBuf
			}
			if e.ExecDetail.Stderr != nil {
				cmdWithTimeout.Stderr = e.ExecDetail.Stderr
			} else {
				cmdWithTimeout.Stderr = &stderrBuf
			}

			err = cmdWithTimeout.Run()
		} else {
			// Execute command without timeout
			err = cmd.Run()
		}

		execOut := &sshexec.ExecOutput{
			Stdout: stdoutBuf.Bytes(),
			Stderr: stderrBuf.Bytes(),
			Err:    err,
		}

		outl.Lock()
		out = append(out, &backends.ExecOutput{
			Output:   execOut,
			Instance: i,
		})
		outl.Unlock()
	})

	return out
}

// shellEscape escapes a string for safe use in shell commands.
func shellEscape(s string) string {
	// Use single quotes and escape any single quotes in the string
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

// InstancesGetSSHKeyPath returns the SSH key path for instances.
func (s *b) InstancesGetSSHKeyPath(instances backends.InstanceList) []string {
	log := s.log.WithPrefix("InstancesGetSSHKeyPath: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	out := []string{}
	for range instances {
		out = append(out, filepath.Join(s.sshKeysDir, s.project))
	}
	return out
}

// InstancesGetSftpConfig returns SFTP configuration for instances.
// For Vagrant, root SSH access is enabled via ssh-key-setup.sh during provisioning.
func (s *b) InstancesGetSftpConfig(instances backends.InstanceList, username string) ([]*sshexec.ClientConf, error) {
	log := s.log.WithPrefix("InstancesGetSftpConfig: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	confs := []*sshexec.ClientConf{}
	for _, i := range instances {
		if i.InstanceState != backends.LifeCycleStateRunning {
			return nil, errors.New("instance not running")
		}

		detail, ok := i.BackendSpecific.(*InstanceDetail)
		if !ok || detail.SSHInfo == nil {
			return nil, errors.New("no SSH info available")
		}

		// Use the aerolab-managed SSH key which has been installed for both vagrant user and root
		// via the ssh-key-setup.sh provisioning script
		keyData, err := os.ReadFile(filepath.Join(s.sshKeysDir, s.project))
		if err != nil {
			// Fallback to Vagrant's key if aerolab key not found
			keyData, err = os.ReadFile(detail.SSHInfo.PrivateKey)
			if err != nil {
				return nil, fmt.Errorf("failed to read SSH key: %w", err)
			}
		}

		clientConf := &sshexec.ClientConf{
			Host:           detail.SSHInfo.Host,
			Port:           detail.SSHInfo.Port,
			Username:       username,
			PrivateKey:     keyData,
			ConnectTimeout: 30 * time.Second,
		}
		confs = append(confs, clientConf)
	}

	return confs, nil
}

// InstancesAssignFirewalls assigns firewalls to instances (not implemented for Vagrant).
func (s *b) InstancesAssignFirewalls(instances backends.InstanceList, fw backends.FirewallList) error {
	log := s.log.WithPrefix("InstancesAssignFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if len(instances) == 0 {
		return nil
	}
	return errors.New("not implemented")
}

// InstancesRemoveFirewalls removes firewalls from instances (not implemented for Vagrant).
func (s *b) InstancesRemoveFirewalls(instances backends.InstanceList, fw backends.FirewallList) error {
	log := s.log.WithPrefix("InstancesRemoveFirewalls: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if len(instances) == 0 {
		return nil
	}
	return errors.New("not implemented")
}

// CreateInstancesGetPrice returns pricing information (always 0 for Vagrant).
func (s *b) CreateInstancesGetPrice(input *backends.CreateInstanceInput) (costPPH, costGB float64, err error) {
	return 0, 0, nil
}

// CreateInstances creates new Vagrant VM instances.
func (s *b) CreateInstances(input *backends.CreateInstanceInput, waitDur time.Duration) (output *backends.CreateInstanceOutput, err error) {
	log := s.log.WithPrefix("CreateInstances: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	s.createInstanceCount.Inc()
	defer s.createInstanceCount.Dec()

	// resolve backend-specific parameters
	backendSpecificParams := &CreateInstanceParams{}
	if input.BackendSpecificParams != nil {
		if _, ok := input.BackendSpecificParams[backends.BackendTypeVagrant]; ok {
			switch input.BackendSpecificParams[backends.BackendTypeVagrant].(type) {
			case *CreateInstanceParams:
				backendSpecificParams = input.BackendSpecificParams[backends.BackendTypeVagrant].(*CreateInstanceParams)
			case CreateInstanceParams:
				item := input.BackendSpecificParams[backends.BackendTypeVagrant].(CreateInstanceParams)
				backendSpecificParams = &item
			default:
				return nil, fmt.Errorf("invalid backend-specific parameters for vagrant")
			}
		}
	}

	if err := structtags.CheckRequired(backendSpecificParams); err != nil {
		return nil, fmt.Errorf("required fields missing in backend-specific parameters: %w", err)
	}

	// Validate parameters
	if backendSpecificParams.CPUs <= 0 {
		return nil, fmt.Errorf("CPUs must be greater than 0, got: %d", backendSpecificParams.CPUs)
	}
	if backendSpecificParams.Memory <= 0 {
		return nil, fmt.Errorf("memory must be greater than 0, got: %d", backendSpecificParams.Memory)
	}
	if input.Nodes <= 0 {
		return nil, fmt.Errorf("nodes must be greater than 0, got: %d", input.Nodes)
	}

	// Determine last node number for this cluster
	lastNodeNo := 0
	clusterUUID := uuid.New().String()
	for _, instance := range s.instances.WithNotState(backends.LifeCycleStateTerminated).WithClusterName(input.ClusterName).Describe() {
		clusterUUID = instance.ClusterUUID
		if instance.NodeNo > lastNodeNo {
			lastNodeNo = instance.NodeNo
		}
	}
	log.Detail("Current last node number in cluster %s: %d", input.ClusterName, lastNodeNo)

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)
	defer s.invalidateCacheFunc(backends.CacheInvalidateVolume)

	// Get working directory for the region
	region := "default"
	if len(s.regions) > 0 {
		region = s.regions[0]
	}
	workDir, err := s.getVagrantWorkDir(region)
	if err != nil {
		return nil, err
	}

	// Ensure work directory exists
	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Create or load SSH key
	sshKeyPath := filepath.Join(s.sshKeysDir, s.project)
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

	// Create instances
	output = &backends.CreateInstanceOutput{
		Instances: backends.InstanceList{},
	}

	var createErrs error
	createErrsLock := new(sync.Mutex)
	wg := new(sync.WaitGroup)
	outputLock := new(sync.Mutex)

	for i := lastNodeNo; i < lastNodeNo+input.Nodes; i++ {
		wg.Add(1)
		go func(nodeIndex int) {
			defer wg.Done()

			vmID := uuid.New().String()
			vmDir := filepath.Join(workDir, vmID)

			if err := os.MkdirAll(vmDir, 0755); err != nil {
				createErrsLock.Lock()
				createErrs = errors.Join(createErrs, fmt.Errorf("failed to create VM directory: %w", err))
				createErrsLock.Unlock()
				return
			}

			// Create metadata
			metadata := make(map[string]string)
			metadata[TAG_OWNER] = input.Owner
			metadata[TAG_CLUSTER_NAME] = input.ClusterName
			metadata[TAG_CLUSTER_UUID] = clusterUUID
			metadata[TAG_NODE_NO] = fmt.Sprintf("%d", nodeIndex+1)
			metadata[TAG_DESCRIPTION] = input.Description
			metadata[TAG_EXPIRES] = input.Expires.Format(time.RFC3339)
			metadata[TAG_AEROLAB_PROJECT] = s.project
			metadata[TAG_AEROLAB_VERSION] = s.aerolabVersion
			metadata[TAG_ARCHITECTURE] = "amd64" // Vagrant typically uses amd64
			metadata["boxName"] = backendSpecificParams.Box
			metadata["boxVersion"] = backendSpecificParams.BoxVersion
			metadata["cpus"] = fmt.Sprintf("%d", backendSpecificParams.CPUs)
			metadata["memory"] = fmt.Sprintf("%d", backendSpecificParams.Memory)
			metadata["createTime"] = time.Now().Format(time.RFC3339)

			// Parse OS name and version from box name
			metadata[TAG_OS_NAME], metadata[TAG_OS_VERSION] = s.parseOSFromBoxName(backendSpecificParams.Box)

			for k, v := range input.Tags {
				metadata[k] = v
			}

			name := input.Name
			if name == "" {
				name = fmt.Sprintf("%s-%s-%d", s.project, input.ClusterName, nodeIndex+1)
			}
			metadata[TAG_NAME] = name

			// Save metadata
			metadataBytes, err := json.MarshalIndent(metadata, "", "  ")
			if err != nil {
				createErrsLock.Lock()
				createErrs = errors.Join(createErrs, fmt.Errorf("failed to marshal metadata: %w", err))
				createErrsLock.Unlock()
				return
			}
			if err := os.WriteFile(filepath.Join(vmDir, "metadata.json"), metadataBytes, 0644); err != nil {
				createErrsLock.Lock()
				createErrs = errors.Join(createErrs, fmt.Errorf("failed to write metadata: %w", err))
				createErrsLock.Unlock()
				return
			}

			// Generate Vagrantfile
			vagrantfile := s.generateVagrantfile(name, backendSpecificParams, string(publicKeyBytes))
			if err := os.WriteFile(filepath.Join(vmDir, "Vagrantfile"), []byte(vagrantfile), 0644); err != nil {
				createErrsLock.Lock()
				createErrs = errors.Join(createErrs, fmt.Errorf("failed to write Vagrantfile: %w", err))
				createErrsLock.Unlock()
				return
			}

			// Start the VM
			log.Detail("Creating VM %s with vagrant up", name)
			cmd := exec.Command("vagrant", "up", "--provider", s.getVagrantProvider())
			cmd.Dir = vmDir
			cmdOutput, err := cmd.CombinedOutput()
			if err != nil {
				createErrsLock.Lock()
				createErrs = errors.Join(createErrs, fmt.Errorf("failed to create VM %s: %w: %s", name, err, string(cmdOutput)))
				createErrsLock.Unlock()
				// Clean up
				os.RemoveAll(vmDir)
				return
			}

			log.Detail("VM %s created successfully", name)

			// Get the created instance details
			instances, err := s.GetInstances(s.volumes, s.networks, s.firewalls)
			if err != nil {
				log.Warn("Failed to refresh instances after creation: %v", err)
				return
			}

			inst := instances.WithInstanceID(vmID)
			if inst.Count() == 1 {
				outputLock.Lock()
				output.Instances = append(output.Instances, inst.Describe()[0])
				outputLock.Unlock()
			}
		}(i)
	}

	wg.Wait()

	if createErrs != nil {
		return nil, createErrs
	}

	// Wait for SSH readiness if requested
	if !backendSpecificParams.SkipSshReadyCheck {
		log.Detail("Waiting for instances to be ssh-ready")
		username := s.getDefaultSSHUsername(backendSpecificParams.Box)

		for waitDur > 0 {
			now := time.Now()
			success := true
			execOut := output.Instances.Exec(&backends.ExecInput{
				Username:        username,
				ParallelThreads: input.ParallelSSHThreads,
				ConnectTimeout:  5 * time.Second,
				ExecDetail: sshexec.ExecDetail{
					Command: []string{"ls", "/"},
				},
			})
			if len(execOut) != len(output.Instances) {
				success = false
			}
			for _, o := range execOut {
				if o.Output.Err != nil {
					success = false
					log.Detail("Waiting for instance %s to be ready: %s", o.Instance.InstanceID, o.Output.Err)
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
	}

	return output, nil
}

// parseOSFromBoxName extracts OS name and version from a Vagrant box name.
func (s *b) parseOSFromBoxName(boxName string) (osName, osVersion string) {
	boxLower := strings.ToLower(boxName)

	// Try to parse common patterns
	if strings.Contains(boxLower, "ubuntu") {
		osName = "ubuntu"
		if strings.Contains(boxLower, "jammy") || strings.Contains(boxLower, "2204") {
			osVersion = "22.04"
		} else if strings.Contains(boxLower, "focal") || strings.Contains(boxLower, "2004") {
			osVersion = "20.04"
		} else if strings.Contains(boxLower, "bionic") || strings.Contains(boxLower, "1804") {
			osVersion = "18.04"
		}
	} else if strings.Contains(boxLower, "centos") {
		osName = "centos"
		if strings.Contains(boxLower, "/7") || strings.Contains(boxLower, "centos7") {
			osVersion = "7"
		} else if strings.Contains(boxLower, "/8") || strings.Contains(boxLower, "centos8") {
			osVersion = "8"
		}
	} else if strings.Contains(boxLower, "rhel") {
		osName = "rhel"
		if strings.Contains(boxLower, "rhel8") || strings.Contains(boxLower, "/8") {
			osVersion = "8"
		} else if strings.Contains(boxLower, "rhel9") || strings.Contains(boxLower, "/9") {
			osVersion = "9"
		}
	} else if strings.Contains(boxLower, "debian") {
		osName = "debian"
		if strings.Contains(boxLower, "bullseye") || strings.Contains(boxLower, "11") {
			osVersion = "11"
		} else if strings.Contains(boxLower, "buster") || strings.Contains(boxLower, "10") {
			osVersion = "10"
		}
	}

	// Default to unknown if we couldn't parse
	if osName == "" {
		osName = "unknown"
		osVersion = "unknown"
	}

	return osName, osVersion
}

// getDefaultSSHUsername returns the default SSH username for a given box.
func (s *b) getDefaultSSHUsername(boxName string) string {
	boxLower := strings.ToLower(boxName)

	// Most official boxes from specific publishers use specific usernames
	if strings.HasPrefix(boxLower, "ubuntu/") {
		return "ubuntu"
	} else if strings.HasPrefix(boxLower, "centos/") {
		return "vagrant"
	} else if strings.HasPrefix(boxLower, "debian/") {
		return "vagrant"
	}

	// Generic boxes typically use "vagrant" user
	return "vagrant"
}

// escapeRubyString escapes quotes in a string for safe use in Ruby double-quoted strings.
func escapeRubyString(s string) string {
	// Escape backslashes first, then double quotes
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "\"", "\\\"")
	// Also escape newlines and other control characters
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// generateVagrantfile generates a Vagrantfile for a VM.
func (s *b) generateVagrantfile(name string, params *CreateInstanceParams, publicKey string) string {
	var vf strings.Builder

	vf.WriteString("Vagrant.configure(\"2\") do |config|\n")
	vf.WriteString(fmt.Sprintf("  config.vm.box = \"%s\"\n", escapeRubyString(params.Box)))
	if params.BoxVersion != "" {
		vf.WriteString(fmt.Sprintf("  config.vm.box_version = \"%s\"\n", escapeRubyString(params.BoxVersion)))
	}
	vf.WriteString(fmt.Sprintf("  config.vm.hostname = \"%s\"\n", escapeRubyString(name)))

	// Network configuration
	// Only configure network if NetworkType and NetworkIP are both provided
	// For private_network without IP, Vagrant requires either an IP or type: "dhcp"
	// We'll skip network configuration if no IP is specified, relying on Vagrant's default NAT
	if params.NetworkType != "" && params.NetworkIP != "" {
		vf.WriteString(fmt.Sprintf("  config.vm.network \"%s\", ip: \"%s\"\n", escapeRubyString(params.NetworkType), escapeRubyString(params.NetworkIP)))
	}

	// Port forwarding
	for guest, host := range params.PortForwards {
		vf.WriteString(fmt.Sprintf("  config.vm.network \"forwarded_port\", guest: %d, host: %d\n", guest, host))
	}

	// Synced folders
	for hostPath, guestPath := range params.SyncedFolders {
		vf.WriteString(fmt.Sprintf("  config.vm.synced_folder \"%s\", \"%s\"\n", escapeRubyString(hostPath), escapeRubyString(guestPath)))
	}

	// Provider-specific configuration
	provider := s.getVagrantProvider()
	vf.WriteString(fmt.Sprintf("  config.vm.provider \"%s\" do |v|\n", provider))
	vf.WriteString(fmt.Sprintf("    v.memory = %d\n", params.Memory))
	vf.WriteString(fmt.Sprintf("    v.cpus = %d\n", params.CPUs))
	if params.DiskSize > 0 {
		vf.WriteString(fmt.Sprintf("    v.disk :disk, size: \"%dGB\", primary: true\n", params.DiskSize))
	}
	vf.WriteString("  end\n")

	// SSH key provisioning - load script from embedded filesystem
	sshKeyScript, err := scripts.ReadFile("scripts/ssh-key-setup.sh")
	if err != nil {
		s.log.Error("Failed to read ssh-key-setup.sh: %v", err)
		// Fallback to inline script
		vf.WriteString("  config.vm.provision \"shell\", inline: <<-SHELL\n")
		vf.WriteString("    mkdir -p /root/.ssh\n")
		vf.WriteString(fmt.Sprintf("    echo '%s' >> /root/.ssh/authorized_keys\n", publicKey))
		vf.WriteString("    chmod 600 /root/.ssh/authorized_keys\n")
		vf.WriteString("    chmod 700 /root/.ssh\n")
		vf.WriteString("    mkdir -p /home/vagrant/.ssh\n")
		vf.WriteString(fmt.Sprintf("    echo '%s' >> /home/vagrant/.ssh/authorized_keys\n", publicKey))
		vf.WriteString("    chown -R vagrant:vagrant /home/vagrant/.ssh\n")
		vf.WriteString("    chmod 600 /home/vagrant/.ssh/authorized_keys\n")
		vf.WriteString("    chmod 700 /home/vagrant/.ssh\n")
		vf.WriteString("  SHELL\n")
	} else {
		scriptContent := strings.ReplaceAll(string(sshKeyScript), "{{PUBLIC_KEY}}", publicKey)
		// Remove the shebang line for inline shell provisioning
		scriptLines := strings.Split(scriptContent, "\n")
		var filteredLines []string
		for _, line := range scriptLines {
			if !strings.HasPrefix(line, "#!") {
				filteredLines = append(filteredLines, line)
			}
		}
		scriptContent = strings.Join(filteredLines, "\n")
		vf.WriteString("  config.vm.provision \"shell\", inline: <<-SHELL\n")
		// Indent each line for proper Ruby heredoc formatting
		for _, line := range strings.Split(scriptContent, "\n") {
			if line != "" {
				vf.WriteString("    " + line + "\n")
			}
		}
		vf.WriteString("  SHELL\n")
	}

	vf.WriteString("end\n")

	return vf.String()
}

// CleanupDNS cleans up stale DNS records (not implemented for Vagrant).
func (s *b) CleanupDNS() error {
	s.log.Detail("CleanupDNS: not implemented")
	return nil
}

// InstancesUpdateHostsFile updates /etc/hosts file on instances.
func (s *b) InstancesUpdateHostsFile(instances backends.InstanceList, hostsEntries []string, parallelSSHThreads int) error {
	log := s.log.WithPrefix("InstancesUpdateHostsFile: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	// Load the update script from embedded filesystem
	scriptContent, err := scripts.ReadFile("scripts/update-hosts.sh")
	if err != nil {
		return fmt.Errorf("failed to read update-hosts.sh: %w", err)
	}
	script := strings.ReplaceAll(string(scriptContent), "{{HOSTS_ENTRIES}}", strings.Join(hostsEntries, "\n"))

	// Upload script to instances using ssh
	sshConfig, err := instances.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get sftp config: %v", err)
	}

	var retErr error
	retErrLock := new(sync.Mutex)
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
				retErrLock.Lock()
				retErr = errors.Join(retErr, fmt.Errorf("failed to create sftp client for host %s: %v", config.Host, err))
				retErrLock.Unlock()
				return
			}
			defer cli.Close()

			err = cli.WriteFile(true, &sshexec.FileWriter{
				DestPath:    "/tmp/update-hosts-file.sh",
				Source:      strings.NewReader(script),
				Permissions: 0755,
			})
			if err != nil {
				retErrLock.Lock()
				retErr = errors.Join(retErr, fmt.Errorf("failed to write update-hosts-file.sh for host %s: %v", config.Host, err))
				retErrLock.Unlock()
				return
			}
		}(config)
	}
	wait.Wait()

	if retErr != nil {
		return retErr
	}

	// Execute script on all instances
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
			log.Detail("ERROR: %s %v", output.Instance.ClusterName+"-"+strconv.Itoa(output.Instance.NodeNo), output.Output.Err)
			errs = errors.Join(errs, fmt.Errorf("failed to update hosts file on instance %s: %v", output.Instance.ClusterName+"-"+strconv.Itoa(output.Instance.NodeNo), output.Output.Err))
		}
	}

	return errs
}

// ResolveNetworkPlacement resolves network placement (not implemented for Vagrant).
func (s *b) ResolveNetworkPlacement(placement string) (vpc *backends.Network, subnet *backends.Subnet, zone string, err error) {
	return nil, nil, "", nil
}

// InstancesChangeExpiry changes the expiry time for instances.
func (s *b) InstancesChangeExpiry(instances backends.InstanceList, expiry time.Time) error {
	log := s.log.WithPrefix("InstancesChangeExpiry: job=" + shortuuid.New() + " ")
	log.Detail("Start")
	defer log.Detail("End")

	if len(instances) == 0 {
		return nil
	}

	defer s.invalidateCacheFunc(backends.CacheInvalidateInstance)

	var errs error
	for _, instance := range instances {
		detail, ok := instance.BackendSpecific.(*InstanceDetail)
		if !ok {
			continue
		}

		// Read current metadata
		metadataFile := filepath.Join(detail.VagrantDir, "metadata.json")
		metadataBytes, err := os.ReadFile(metadataFile)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to read metadata for %s: %w", instance.Name, err))
			continue
		}

		var metadata map[string]string
		if err := json.Unmarshal(metadataBytes, &metadata); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to parse metadata for %s: %w", instance.Name, err))
			continue
		}

		// Update expiry
		metadata[TAG_EXPIRES] = expiry.Format(time.RFC3339)

		// Write updated metadata
		updatedBytes, err := json.MarshalIndent(metadata, "", "  ")
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to marshal metadata for %s: %w", instance.Name, err))
			continue
		}

		if err := os.WriteFile(metadataFile, updatedBytes, 0644); err != nil {
			errs = errors.Join(errs, fmt.Errorf("failed to write metadata for %s: %w", instance.Name, err))
			continue
		}
	}

	return errs
}
