package cmd

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/backend/clouds/bdocker"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/docker/docker/api/types/strslice"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClientCreateGraphCmd struct {
	ClientName         TypeClientName           `short:"n" long:"group-name" description:"Client group name" default:"client"`
	ClientCount        int                      `short:"c" long:"count" description:"Number of clients" default:"1"`
	Owner              string                   `short:"o" long:"owner" description:"Owner of the instances"`
	AWS                InstancesCreateCmdAws    `group:"AWS" description:"backend-aws" namespace:"aws"`
	GCP                InstancesCreateCmdGcp    `group:"GCP" description:"backend-gcp" namespace:"gcp"`
	Docker             InstancesCreateCmdDocker `group:"Docker" description:"backend-docker" namespace:"docker"`
	SeedClusterName    TypeClusterName          `short:"C" long:"cluster-name" description:"Cluster name to seed from" default:"mydc"`
	Seed               string                   `long:"seed" description:"Specify a seed IP:PORT instead of providing a ClusterName; if this parameter is provided, ClusterName is ignored"`
	Namespace          string                   `short:"m" long:"namespace" description:"Namespace name to configure graph to use" default:"test"`
	ExtraProperties    []string                 `short:"e" long:"extra" description:"Extra properties to add; can be specified multiple times; ex: -e 'aerospike.client.timeout=2000'"`
	AMS                string                   `long:"ams" description:"Name of an AMS client to add this machine to prometheus configs to"`
	RAMMb              int                      `long:"ram-mb" description:"Manually specify amount of RAM MiB to use"`
	GraphImage         string                   `long:"graph-image" description:"Docker image to use for graph installation" default:"aerospike/aerospike-graph-service"`
	DockerLoginUser    string                   `long:"docker-user" description:"Login to docker registry for graph installation"`
	DockerLoginPass    string                   `long:"docker-pass" description:"Login to docker registry for graph installation" webtype:"password"`
	DockerLoginURL     string                   `long:"docker-url" description:"Login to docker registry for graph installation"`
	GraphPrivileged    bool                     `long:"graph-privileged" description:"Force graph to run in privileged docker container mode"`
	JustDoIt           bool                     `long:"confirm" description:"Confirm any warning questions without being asked" webdisable:"true" webset:"true"`
	TypeOverride       string                   `long:"type-override" description:"Override the client type label"`
	ParallelSSHThreads int                      `long:"threads" description:"Number of threads to use for the execution" default:"10"`
	Help               HelpCmd                  `command:"help" subcommands-optional:"true" description:"Print help"`
}

// GraphConfigParams holds parameters for generating graph configuration
type GraphConfigParams struct {
	Seeds           []string
	Namespace       string
	ExtraProperties []string
	RAMMb           int
}

// GraphInstallParams holds parameters for the graph installation script
type GraphInstallParams struct {
	ConfigPath      string
	GraphImage      string
	GraphPrivileged bool
	DockerLoginUser string
	DockerLoginPass string
	DockerLoginURL  string
}

func (c *ClientCreateGraphCmd) Execute(args []string) error {
	isGrow := len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow"

	var cmd []string
	if isGrow {
		cmd = []string{"client", "grow", "graph"}
	} else {
		cmd = []string{"client", "create", "graph"}
	}

	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.createGraphClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateGraphCmd) createGraphClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "create", "graph"}, c)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Override type
	if c.TypeOverride == "" {
		c.TypeOverride = "graph"
	}

	// Resolve seed IP and port
	seedIP, seedPort, err := c.resolveSeed(system, inventory, logger)
	if err != nil {
		return err
	}

	// Verify AMS client exists if specified
	if c.AMS != "" {
		amsClients := inventory.Instances.WithTags(map[string]string{"aerolab.old.type": "client"}).WithClusterName(c.AMS)
		if amsClients == nil || amsClients.Count() == 0 {
			return fmt.Errorf("AMS client '%s' not found", c.AMS)
		}
	}

	// Handle Docker backend specifics
	if system.Opts.Config.Backend.Type == "docker" {
		return c.createGraphOnDocker(system, inventory, logger, args, isGrow, seedIP, seedPort)
	}

	// Handle cloud backend (AWS/GCP)
	return c.createGraphOnCloud(system, inventory, logger, args, isGrow, seedIP, seedPort)
}

// resolveSeed resolves the seed IP and port from either direct seed parameter or cluster name
func (c *ClientCreateGraphCmd) resolveSeed(system *System, inventory *backends.Inventory, logger *logger.Logger) (string, string, error) {
	if c.Seed != "" {
		// Parse direct seed parameter
		addr, err := net.ResolveTCPAddr("tcp", c.Seed)
		if err != nil {
			return "", "", fmt.Errorf("failed to resolve seed address '%s': %w", c.Seed, err)
		}
		return addr.IP.String(), strconv.Itoa(addr.Port), nil
	}

	// Get seed from cluster
	logger.Info("Getting cluster %s information", c.SeedClusterName.String())

	cluster := inventory.Instances.WithClusterName(c.SeedClusterName.String()).WithState(backends.LifeCycleStateRunning)
	if cluster == nil || cluster.Count() == 0 {
		return "", "", fmt.Errorf("cluster '%s' not found or has no running instances", c.SeedClusterName.String())
	}

	instances := cluster.Describe()

	// Find a node with an IP
	var seedIP string
	for _, inst := range instances {
		ip := inst.IP.Private
		if ip == "" {
			ip = inst.IP.Public
		}
		if ip != "" {
			seedIP = ip
			break
		}
	}

	if seedIP == "" {
		return "", "", fmt.Errorf("could not find an IP for a node in cluster '%s' - are all the nodes down?", c.SeedClusterName.String())
	}

	seedPort := "3000"

	// For Docker backend, check for exposed ports
	if system.Opts.Config.Backend.Type == "docker" {
		for _, inst := range instances {
			if inst.Tags != nil {
				// Look for docker exposed ports in tags or other metadata
				if exposedPort, ok := inst.Tags["aerolab.docker.aerospike.port"]; ok && exposedPort != "" {
					seedPort = exposedPort
					if inst.IP.Private != "" {
						seedIP = inst.IP.Private
					}
					break
				}
			}
		}
	}

	logger.Info("Using seed %s:%s from cluster %s", seedIP, seedPort, c.SeedClusterName.String())
	return seedIP, seedPort, nil
}

// createGraphOnDocker handles graph client creation on Docker backend
func (c *ClientCreateGraphCmd) createGraphOnDocker(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool, seedIP, seedPort string) error {
	// Docker backend uses custom docker image directly
	if c.ClientCount > 1 {
		return fmt.Errorf("on docker backend, only one graph client can be deployed at a time")
	}

	// Default RAM for Docker deployment
	if c.RAMMb == 0 {
		logger.Info("For local docker deployment, defaulting to 4G RAM limit")
		c.RAMMb = 4096
	}

	// Handle port exposure warning
	hasPort8182 := false
	for _, port := range c.Docker.ExposePorts {
		if strings.Contains(port, ":8182") {
			hasPort8182 = true
			break
		}
	}
	if !hasPort8182 {
		logger.Info("Docker backend is in use. Auto-exposing port 8182 for graph access.")
		c.Docker.ExposePorts = append(c.Docker.ExposePorts, "+8182:8182")
	}

	// Generate graph configuration
	graphConfig, err := c.generateGraphConfig([]string{seedIP + ":" + seedPort})
	if err != nil {
		return fmt.Errorf("failed to generate graph configuration: %w", err)
	}

	// Create temporary config file
	tmpDir, err := os.MkdirTemp("", "aerolab-graph-*")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	confFile := fmt.Sprintf("%s/aerospike-graph.properties", tmpDir)
	if err := os.WriteFile(confFile, graphConfig, 0644); err != nil {
		return fmt.Errorf("failed to write configuration file: %w", err)
	}
	logger.Info("Saved configuration to %s", confFile)

	// Set up Docker-specific parameters
	c.Docker.ImageName = c.GraphImage
	c.Docker.Privileged = c.GraphPrivileged || c.Docker.Privileged

	// Add volume mount for config file
	c.Docker.Disks = append(c.Docker.Disks, confFile+":/opt/aerospike-graph/aerospike-graph.properties")

	// Set RAM limit if specified
	if c.RAMMb > 0 {
		// Get advanced config or create new one
		memoryBytes := int64(c.RAMMb) * 1024 * 1024
		// We need to update the Docker params to include memory limit
		// This will be handled by the InstancesCreateCmd through advanced config
		logger.Debug("Setting memory limit to %d MiB", c.RAMMb)

		// Create a temporary advanced config for memory limit
		advancedConfigFile := fmt.Sprintf("%s/docker-advanced.json", tmpDir)
		advancedConfig := fmt.Sprintf(`{"resources":{"memory":%d}}`, memoryBytes)
		if err := os.WriteFile(advancedConfigFile, []byte(advancedConfig), 0644); err != nil {
			return fmt.Errorf("failed to write advanced config: %w", err)
		}
		c.Docker.AdvancedConfigPath = flags.Filename(advancedConfigFile)
	}

	// Pass docker registry credentials to the Docker backend for image pulling
	if c.DockerLoginUser != "" && c.DockerLoginPass != "" {
		logger.Info("Docker registry credentials provided, will use for image pull")
		c.Docker.RegistryUser = c.DockerLoginUser
		c.Docker.RegistryPass = c.DockerLoginPass
		c.Docker.RegistryURL = c.DockerLoginURL
	}

	logger.Info("Pulling and running dockerized aerospike-graph, this may take a while...")

	// Create instances using InstancesCreateCmd with custom docker image
	instancesCmd := InstancesCreateCmd{
		ClusterName:        c.ClientName.String(),
		Count:              c.ClientCount,
		Owner:              c.Owner,
		Type:               c.TypeOverride,
		Tags:               []string{"aerolab.old.type=client", "aerolab.client.type=graph"},
		OS:                 "ubuntu",
		Version:            "24.04",
		Arch:               "amd64",
		Docker:             c.Docker,
		ParallelSSHThreads: c.ParallelSSHThreads,
	}

	// We need special handling for custom docker image in docker backend
	// The InstancesCreateCmd handles custom docker images via Docker.ImageName
	_, err = instancesCmd.CreateInstances(system, inventory, args, func() string {
		if isGrow {
			return "grow"
		}
		return "create"
	}())
	if err != nil {
		return fmt.Errorf("failed to create graph client: %w", err)
	}

	// Configure AMS if specified
	if c.AMS != "" {
		if err := c.configureAMS(system, inventory, logger); err != nil {
			logger.Warn("Failed to configure AMS: %s", err)
		}
	}

	c.printDockerInstructions(logger)
	return nil
}

// createGraphOnCloud handles graph client creation on AWS/GCP backends
func (c *ClientCreateGraphCmd) createGraphOnCloud(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool, seedIP, seedPort string) error {
	// Create base client first
	baseCmd := &ClientCreateBaseCmd{ClientCreateNoneCmd: ClientCreateNoneCmd{
		ClientName:         c.ClientName,
		ClientCount:        c.ClientCount,
		Owner:              c.Owner,
		AWS:                c.AWS,
		GCP:                c.GCP,
		Docker:             c.Docker,
		OS:                 "ubuntu",
		Version:            "24.04",
		Arch:               "amd64",
		TypeOverride:       c.TypeOverride,
		ParallelSSHThreads: c.ParallelSSHThreads,
	}}
	clients, err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return fmt.Errorf("failed to create base client: %w", err)
	}

	logger.Info("Continuing graph installation on %d instances...", len(clients))

	// Generate graph configuration
	graphConfig, err := c.generateGraphConfig([]string{seedIP + ":" + seedPort})
	if err != nil {
		return fmt.Errorf("failed to generate graph configuration: %w", err)
	}

	// Generate install script
	installScript, err := c.generateInstallScript()
	if err != nil {
		return fmt.Errorf("failed to generate install script: %w", err)
	}

	// Install graph on each client in parallel
	errs := parallelize.MapLimit(clients.Describe(), c.ParallelSSHThreads, func(client *backends.Instance) error {
		return c.installGraphOnInstance(client, graphConfig, installScript, logger, system.logLevel)
	})

	// Check for errors
	var hasErr bool
	for i, err := range errs {
		if err != nil {
			logger.Error("Node %d returned error: %s", clients.Describe()[i].NodeNo, err)
			hasErr = true
		}
	}
	if hasErr {
		return fmt.Errorf("some nodes returned errors during graph installation")
	}

	// Configure AMS if specified
	if c.AMS != "" {
		if err := c.configureAMS(system, inventory, logger); err != nil {
			logger.Warn("Failed to configure AMS: %s", err)
		}
	}

	c.printCloudInstructions(logger)
	return nil
}

// installGraphOnInstance installs graph on a single cloud instance
func (c *ClientCreateGraphCmd) installGraphOnInstance(client *backends.Instance, graphConfig, installScript []byte, logger *logger.Logger, logLevel logger.LogLevel) error {
	// Get SFTP config
	conf, err := client.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	// Create SFTP client
	sftpClient, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// Upload config file
	err = sftpClient.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/etc/aerospike-graph.properties",
		Source:      bytes.NewReader(graphConfig),
		Permissions: 0644,
	})
	if err != nil {
		return fmt.Errorf("failed to upload graph configuration: %w", err)
	}

	// Upload install script
	err = sftpClient.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/tmp/install-graph.sh",
		Source:      bytes.NewReader(installScript),
		Permissions: 0755,
	})
	if err != nil {
		return fmt.Errorf("failed to upload install script: %w", err)
	}

	// Execute install script
	var stdout, stderr *os.File
	var stdin io.ReadCloser
	terminal := false
	if logLevel >= 5 {
		stdout = os.Stdout
		stderr = os.Stderr
		stdin = io.NopCloser(os.Stdin)
		terminal = true
	}

	execDetail := sshexec.ExecDetail{
		Command:        []string{"bash", "/tmp/install-graph.sh"},
		SessionTimeout: 30 * time.Minute,
		Terminal:       terminal,
	}
	if logLevel >= 5 {
		execDetail.Stdin = stdin
		execDetail.Stdout = stdout
		execDetail.Stderr = stderr
	}

	output := client.Exec(&backends.ExecInput{
		ExecDetail:     execDetail,
		Username:       "root",
		ConnectTimeout: 30 * time.Second,
	})

	if output.Output.Err != nil {
		return fmt.Errorf("install script failed: %w (stdout: %s, stderr: %s)",
			output.Output.Err, output.Output.Stdout, output.Output.Stderr)
	}

	logger.Info("Successfully installed graph on %s:%d", client.ClusterName, client.NodeNo)
	return nil
}

// generateGraphConfig generates the graph properties configuration
func (c *ClientCreateGraphCmd) generateGraphConfig(seeds []string) ([]byte, error) {
	params := GraphConfigParams{
		Seeds:           seeds,
		Namespace:       c.Namespace,
		ExtraProperties: c.ExtraProperties,
		RAMMb:           c.RAMMb,
	}

	graphConfigTemplate, err := scripts.ReadFile("scripts/graph-config.tpl")
	if err != nil {
		return nil, fmt.Errorf("failed to read graph config template: %w", err)
	}

	tmpl, err := template.New("graphConfig").Parse(string(graphConfigTemplate))
	if err != nil {
		return nil, fmt.Errorf("failed to parse graph config template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return nil, fmt.Errorf("failed to execute graph config template: %w", err)
	}

	return buf.Bytes(), nil
}

// generateInstallScript generates the graph installation script for cloud instances
func (c *ClientCreateGraphCmd) generateInstallScript() ([]byte, error) {
	params := GraphInstallParams{
		ConfigPath:      "/etc/aerospike-graph.properties",
		GraphImage:      c.GraphImage,
		GraphPrivileged: c.GraphPrivileged,
		DockerLoginUser: c.DockerLoginUser,
		DockerLoginPass: c.DockerLoginPass,
		DockerLoginURL:  c.DockerLoginURL,
	}

	graphInstallScriptTemplate, err := scripts.ReadFile("scripts/graph-install.sh.tpl")
	if err != nil {
		return nil, fmt.Errorf("failed to read install script template: %w", err)
	}

	tmpl, err := template.New("graphInstall").Parse(string(graphInstallScriptTemplate))
	if err != nil {
		return nil, fmt.Errorf("failed to parse install script template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, params); err != nil {
		return nil, fmt.Errorf("failed to execute install script template: %w", err)
	}

	return buf.Bytes(), nil
}

// configureAMS configures AMS to monitor graph clients
func (c *ClientCreateGraphCmd) configureAMS(system *System, inventory *backends.Inventory, logger *logger.Logger) error {
	logger.Info("Configuring AMS client '%s' to monitor graph client '%s'", c.AMS, c.ClientName.String())

	amsCmd := &ClientConfigureAMSCmd{
		ClientName:     TypeClientName(c.AMS),
		ConnectClients: c.ClientName,
	}

	return amsCmd.configureAMS(system, inventory, logger, nil)
}

// printDockerInstructions prints helpful instructions for Docker deployment
func (c *ClientCreateGraphCmd) printDockerInstructions(logger *logger.Logger) {
	logger.Info("")
	logger.Info("Common tasks and commands:")
	logger.Info(" * access gremlin console:          docker run -it --rm tinkerpop/gremlin-console")
	logger.Info(" * access terminal on graph server: aerolab attach client -n %s", c.ClientName.String())
	logger.Info(" * visit https://gdotv.com/ to download a Graph IDE and Visualization tool")
}

// printCloudInstructions prints helpful instructions for cloud deployment
func (c *ClientCreateGraphCmd) printCloudInstructions(logger *logger.Logger) {
	logger.Info("")
	logger.Info("Common tasks and commands:")
	logger.Info(" * access gremlin console locally:         docker run -it --rm tinkerpop/gremlin-console")
	logger.Info(" * access gremlin console on graph server: aerolab attach client -n %s -- docker run -it --rm tinkerpop/gremlin-console", c.ClientName.String())
	logger.Info(" * access terminal on graph server:        aerolab attach client -n %s", c.ClientName.String())
	logger.Info(" * visit https://gdotv.com/ to download a Graph IDE and Visualization tool")
	logger.Info("")
	logger.Info("Example creating a dedicated gremlin-console client:")
	logger.Info(" * create an empty client: aerolab client create base -n mygremlin [...]")
	logger.Info(" * download docker script: aerolab client attach -n mygremlin -- curl -fsSL https://get.docker.com -o /tmp/get-docker.sh")
	logger.Info(" * install docker        : aerolab client attach -n mygremlin -- bash /tmp/get-docker.sh")
	logger.Info(" * run gremlin-console   : aerolab client attach -n mygremlin -- docker run -it --rm tinkerpop/gremlin-console")
}

// Ensure ClientCreateGraphCmd implements the necessary interface for custom Docker images
func init() {
	// Register any necessary hooks for the graph command
	_ = bdocker.CreateInstanceParams{}
	_ = strslice.StrSlice{}
}
