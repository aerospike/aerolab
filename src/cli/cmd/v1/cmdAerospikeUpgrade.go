package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	"github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type AerospikeUpgradeCmd struct {
	ClusterName      TypeClusterName `short:"n" long:"name" description:"Cluster names, comma separated" default:"mydc"`
	Nodes            TypeNodes       `short:"l" long:"nodes" description:"Nodes list, comma separated. Empty=ALL" default:""`
	AerospikeVersion string          `short:"v" long:"aerospike-version" description:"Aerospike server version to upgrade to; add 'c' to the end for community edition, or 'f' for federal edition" default:"latest"`
	CustomSourceFile flags.Filename  `short:"f" long:"custom-source-file" description:"custom source file for upgrade; must be .deb, .rpm, .tgz, or the asd binary itself"`
	RestartAerospike bool            `short:"r" long:"restart" description:"Restart aerospike service after upgrade"`
	Threads          int             `short:"t" long:"threads" description:"Threads to use" default:"10"`
	Help             HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *AerospikeUpgradeCmd) Execute(args []string) error {
	cmd := []string{"aerospike", "upgrade"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	instances, err := c.UpgradeAerospike(system, system.Backend.GetInventory(), system.Logger, args, "upgrade")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Upgraded aerospike on %d instances", instances.Count())
	for _, i := range instances.Describe() {
		system.Logger.Debug("clusterName=%s nodeNo=%d instanceName=%s instanceID=%s", i.ClusterName, i.NodeNo, i.Name, i.InstanceID)
	}
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// UpgradeAerospike upgrades Aerospike to the specified version on the cluster nodes.
// This function performs the following operations:
// 1. Resolves the Aerospike version and gets the install script
// 2. Creates an upgrade script with optional restart functionality
// 3. Uploads and executes the script on each node
// 4. Handles debug output and error reporting
//
// Parameters:
//   - system: The system instance for logging and backend operations
//   - inventory: The backend inventory containing cluster information
//   - logger: Logger instance for output
//   - args: Command line arguments
//   - action: The action being performed
//
// Returns:
//   - backends.InstanceList: List of instances that were processed
//   - error: nil on success, or an error describing what failed
func (c *AerospikeUpgradeCmd) UpgradeAerospike(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, action string) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"aerospike", action}, c, args...)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	if c.ClusterName.String() == "" {
		return nil, fmt.Errorf("cluster name is required")
	}
	if strings.Contains(c.ClusterName.String(), ",") {
		clusters := strings.Split(c.ClusterName.String(), ",")
		var instances backends.InstanceList
		for _, cluster := range clusters {
			c.ClusterName = TypeClusterName(cluster)
			inst, err := c.UpgradeAerospike(system, inventory, logger, args, action)
			if err != nil {
				return nil, err
			}
			instances = append(instances, inst...)
		}
		return instances, nil
	}

	cluster := inventory.Instances.WithClusterName(c.ClusterName.String())
	if cluster == nil {
		return nil, fmt.Errorf("cluster %s not found", c.ClusterName.String())
	}

	if c.Nodes.String() != "" {
		nodes, err := expandNodeNumbers(c.Nodes.String())
		if err != nil {
			return nil, err
		}
		cluster = cluster.WithNodeNo(nodes...).WithState(backends.LifeCycleStateRunning)
		if cluster.Count() != len(nodes) {
			return nil, fmt.Errorf("some nodes in %s not found", c.Nodes.String())
		}
	}

	cluster = cluster.WithState(backends.LifeCycleStateRunning)
	if cluster.Count() == 0 {
		logger.Info("No running instances found for cluster %s", c.ClusterName.String())
		return nil, nil
	}

	// Check if custom source file is provided
	if string(c.CustomSourceFile) != "" {
		return c.customUpgrade(cluster.Describe(), system, logger)
	}

	// Resolve Aerospike version and get install script
	version, _, err := c.resolveAerospikeVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to resolve aerospike version: %w", err)
	}
	logger.Info("Upgrading aerospike to version %s on %d nodes", version.Name, cluster.Count())

	// Get the install script for the version
	installScript, err := c.getInstallScript(version, cluster.Describe()[0], system, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to get install script: %w", err)
	}

	// Create upgrade script with optional restart functionality
	upgradeScript, err := c.createUpgradeScript(installScript, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create upgrade script: %w", err)
	}

	// Output script contents if debug level is selected
	if system.logLevel >= 5 {
		logger.Info("Upgrade script contents:")
		fmt.Fprintf(os.Stdout, "%s\n", upgradeScript)
	}

	// Process each instance
	var hasErr error
	var errMutex sync.Mutex
	parallelize.ForEachLimit(cluster.Describe(), c.Threads, func(instance *backends.Instance) {
		err := c.upgradeInstance(instance, upgradeScript, system, logger)
		if err != nil {
			errMutex.Lock()
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", instance.ClusterName, instance.NodeNo, err))
			errMutex.Unlock()
			logger.Error("Failed to upgrade instance %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
		}
	})

	return cluster.Describe(), hasErr
}

// resolveAerospikeVersion resolves the Aerospike version and determines the flavor.
func (c *AerospikeUpgradeCmd) resolveAerospikeVersion() (*aerospike.Version, string, error) {
	// Find and resolve aerospike version
	products, err := aerospike.GetProducts(time.Second * 10)
	if err != nil {
		return nil, "", fmt.Errorf("could not get products: %w", err)
	}
	products = products.WithNamePrefix("aerospike-server-")

	flavor := "enterprise"
	if strings.HasSuffix(c.AerospikeVersion, "c") {
		products = products.WithNameSuffix("-community")
		flavor = "community"
	} else if strings.HasSuffix(c.AerospikeVersion, "f") {
		products = products.WithNameSuffix("-federal")
		flavor = "federal"
	} else {
		products = products.WithNameSuffix("-enterprise")
		flavor = "enterprise"
	}

	if len(products) == 0 {
		return nil, "", fmt.Errorf("aerospike version %s not found", c.AerospikeVersion)
	}

	versions, err := aerospike.GetVersions(time.Second*10, products[0])
	if err != nil {
		return nil, "", fmt.Errorf("could not get versions: %w", err)
	}

	if strings.HasPrefix(c.AerospikeVersion, "latest") {
		// Find the latest version
		if len(versions) == 0 {
			return nil, "", fmt.Errorf("no versions found for aerospike %s", flavor)
		}
		return &versions[0], flavor, nil
	}

	// Handle partial version matching (e.g., "8.*" or "8.")
	versionName := c.AerospikeVersion
	if flavor != "enterprise" {
		versionName = strings.TrimSuffix(versionName, flavor[:1])
	}

	if strings.HasSuffix(versionName, "*") || strings.HasSuffix(versionName, ".") {
		// Use prefix matching for partial versions
		prefix := strings.TrimSuffix(versionName, "*")
		matchingVersions := versions.WithNamePrefix(prefix)
		if len(matchingVersions) == 0 {
			return nil, "", fmt.Errorf("aerospike version %s not found", c.AerospikeVersion)
		}
		return &matchingVersions[0], flavor, nil
	}

	// Find exact version match
	for _, version := range versions {
		if version.Name == versionName {
			return &version, flavor, nil
		}
	}

	return nil, "", fmt.Errorf("aerospike version %s not found", c.AerospikeVersion)
}

// getInstallScript gets the install script for the specified version.
func (c *AerospikeUpgradeCmd) getInstallScript(version *aerospike.Version, instance *backends.Instance, system *System, logger *logger.Logger) ([]byte, error) {
	// Get the installer files
	files, err := aerospike.GetFiles(time.Second*10, *version)
	if err != nil {
		return nil, fmt.Errorf("could not get files: %w", err)
	}
	// Get the install script (download=true, install=true, upgrade=true)
	arch := aerospike.ArchitectureTypeUnknown
	switch instance.Architecture {
	case backends.ArchitectureX8664:
		arch = aerospike.ArchitectureTypeX86_64
	case backends.ArchitectureARM64:
		arch = aerospike.ArchitectureTypeAARCH64
	}
	logger.Detail("Architecture: %s, OS Name: %s, OS Version: %s", arch, instance.OperatingSystem.Name, instance.OperatingSystem.Version)
	installScript, err := files.GetInstallScript(
		arch,
		aerospike.OSName(instance.OperatingSystem.Name),
		instance.OperatingSystem.Version,
		system.logLevel >= 5,
		true,
		true,
		true,
	)
	if err != nil {
		return nil, fmt.Errorf("could not get install script: %w", err)
	}

	return installScript, nil
}

// createUpgradeScript creates the upgrade script with optional restart functionality.
func (c *AerospikeUpgradeCmd) createUpgradeScript(installScript []byte, logger *logger.Logger) ([]byte, error) {
	var script bytes.Buffer

	// Add shebang
	script.WriteString("#!/bin/bash\n")
	script.WriteString("set -e\n\n")

	// Add restart at beginning if requested
	if c.RestartAerospike {
		script.WriteString("# Stop aerospike service\n")
		script.WriteString("systemctl stop aerospike || true\n\n")
	}

	// Add the install script
	script.Write(installScript)

	// Add restart at end if requested
	if c.RestartAerospike {
		script.WriteString("\n# Start aerospike service\n")
		script.WriteString("systemctl start aerospike\n")
	}

	return script.Bytes(), nil
}

// upgradeInstance upgrades Aerospike on a single instance.
func (c *AerospikeUpgradeCmd) upgradeInstance(instance *backends.Instance, upgradeScript []byte, system *System, logger *logger.Logger) error {
	// Get SFTP configuration
	conf, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	// Create SFTP client
	client, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer client.Close()

	// Upload upgrade script
	err = client.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/tmp/upgrade-aerospike.sh",
		Source:      bytes.NewReader(upgradeScript),
		Permissions: 0755,
	})
	if err != nil {
		return fmt.Errorf("failed to upload upgrade script: %w", err)
	}

	logger.Info("Running upgrade script on %s:%d", instance.ClusterName, instance.NodeNo)

	// Execute the upgrade script
	var stdout, stderr *os.File
	var stdin *io.ReadCloser
	terminal := false

	// If debug level is selected, output to stdout/stderr
	if system.logLevel >= 5 {
		stdout = os.Stdout
		stderr = os.Stderr
		terminal = true
		stdinp := io.NopCloser(os.Stdin)
		stdin = &stdinp
	}
	detail := sshexec.ExecDetail{
		Command:        []string{"bash", "/tmp/upgrade-aerospike.sh"},
		Terminal:       terminal,
		SessionTimeout: 15 * time.Minute,
	}
	if stdin != nil {
		detail.Stdin = *stdin
	}
	if stdout != nil {
		detail.Stdout = stdout
	}
	if stderr != nil {
		detail.Stderr = stderr
	}
	output := instance.Exec(&backends.ExecInput{
		ExecDetail:      detail,
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})

	// Check for errors
	if output.Output.Err != nil {
		// Output script contents on failure if not already shown
		if system.logLevel < 5 {
			logger.Error("Upgrade script failed, script contents:")
			fmt.Fprintf(os.Stderr, "%s\n", upgradeScript)
		}
		return fmt.Errorf("upgrade script failed: %w\nstdout: %s\nstderr: %s",
			output.Output.Err,
			string(output.Output.Stdout),
			string(output.Output.Stderr))
	}

	logger.Info("Successfully upgraded aerospike on %s:%d", instance.ClusterName, instance.NodeNo)
	return nil
}

// customUpgrade performs an upgrade using a custom source file (.deb, .rpm, .tgz, or asd binary).
// This function performs the following operations:
// 1. Validates the custom source file extension
// 2. Uploads the file to each node
// 3. Backs up the existing aerospike.conf
// 4. Stops aerospike if restart is requested
// 5. Installs the upgrade based on file type
// 6. Restores the aerospike.conf
// 7. Starts aerospike if restart is requested
//
// Parameters:
//   - cluster: The instance list to upgrade
//   - system: The system instance for logging and backend operations
//   - logger: Logger instance for output
//
// Returns:
//   - backends.InstanceList: List of instances that were processed
//   - error: nil on success, or an error describing what failed
func (c *AerospikeUpgradeCmd) customUpgrade(cluster backends.InstanceList, system *System, logger *logger.Logger) (backends.InstanceList, error) {
	customFile := string(c.CustomSourceFile)

	// Validate file extension
	var dstType string
	if strings.HasSuffix(customFile, ".deb") {
		dstType = "upgrade.deb"
	} else if strings.HasSuffix(customFile, ".rpm") {
		dstType = "upgrade.rpm"
	} else if strings.HasSuffix(customFile, ".tgz") {
		dstType = "upgrade.tgz"
	} else if strings.HasSuffix(customFile, "asd") {
		dstType = "asd"
	} else {
		return nil, errors.New("custom source file must be .deb, .rpm, .tgz, or the asd binary itself")
	}

	// Validate file exists and get contents
	stat, err := os.Stat(customFile)
	if err != nil {
		return nil, fmt.Errorf("failed to stat custom source file: %w", err)
	}
	fileSize := stat.Size()

	logger.Info("Upgrading aerospike using custom source file: %s (%d bytes)", customFile, fileSize)

	// Process each instance
	var hasErr error
	var errMutex sync.Mutex
	ntime := strconv.Itoa(int(time.Now().Unix()))

	parallelize.ForEachLimit(cluster.Describe(), c.Threads, func(instance *backends.Instance) {
		err := c.customUpgradeInstance(instance, customFile, dstType, ntime, system, logger)
		if err != nil {
			errMutex.Lock()
			hasErr = errors.Join(hasErr, fmt.Errorf("%s:%d: %w", instance.ClusterName, instance.NodeNo, err))
			errMutex.Unlock()
			logger.Error("Failed to upgrade instance %s:%d: %s", instance.ClusterName, instance.NodeNo, err)
		}
	})

	return cluster.Describe(), hasErr
}

// customUpgradeInstance upgrades Aerospike on a single instance using a custom source file.
func (c *AerospikeUpgradeCmd) customUpgradeInstance(instance *backends.Instance, customFile string, dstType string, ntime string, system *System, logger *logger.Logger) error {
	// Get SFTP configuration
	conf, err := instance.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	// Create SFTP client
	client, err := sshexec.NewSftp(conf)
	if err != nil {
		return fmt.Errorf("failed to create SFTP client: %w", err)
	}
	defer client.Close()

	// Open the custom source file
	fnContents, err := os.Open(customFile)
	if err != nil {
		return fmt.Errorf("failed to open custom source file: %w", err)
	}
	defer fnContents.Close()

	// Upload the custom source file
	logger.Info("Uploading %s to %s:%d", customFile, instance.ClusterName, instance.NodeNo)
	err = client.WriteFile(true, &sshexec.FileWriter{
		DestPath:    "/root/" + dstType,
		Source:      fnContents,
		Permissions: 0644,
	})
	if err != nil {
		return fmt.Errorf("failed to upload custom source file: %w", err)
	}

	// Backup aerospike.conf
	var confBackup bytes.Buffer
	err = client.ReadFile(&sshexec.FileReader{
		SourcePath:  "/etc/aerospike/aerospike.conf",
		Destination: &confBackup,
	})
	if err != nil {
		return fmt.Errorf("failed to backup aerospike.conf: %w", err)
	}

	// Stop aerospike if restart is requested
	if c.RestartAerospike {
		logger.Info("Stopping aerospike on %s:%d", instance.ClusterName, instance.NodeNo)
		output := instance.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"systemctl", "stop", "aerospike"},
				SessionTimeout: 2 * time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if output.Output.Err != nil {
			logger.Warn("Failed to stop aerospike on %s:%d: %s", instance.ClusterName, instance.NodeNo, output.Output.Err)
		}
	}

	// Perform the upgrade based on file type
	logger.Info("Installing upgrade on %s:%d", instance.ClusterName, instance.NodeNo)
	var installCmd []string
	switch dstType {
	case "upgrade.deb":
		installCmd = []string{"/bin/bash", "-c", "export DEBIAN_FRONTEND=noninteractive; apt-get update && apt-get install -y /root/upgrade.deb"}
	case "upgrade.rpm":
		installCmd = []string{"yum", "-y", "localinstall", "/root/upgrade.rpm"}
	case "asd":
		installCmd = []string{"/bin/bash", "-c", "mv /root/asd /usr/bin/asd && chmod 755 /usr/bin/asd"}
	case "upgrade.tgz":
		// First create temp directory and extract
		mkdirCmd := []string{"mkdir", "-p", "/tmp/" + ntime}
		output := instance.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        mkdirCmd,
				SessionTimeout: time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to create temp directory: %w", output.Output.Err)
		}

		// Extract tarball
		tarCmd := []string{"tar", "-zxvf", "/root/upgrade.tgz", "-C", "/tmp/" + ntime}
		output = instance.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        tarCmd,
				SessionTimeout: 5 * time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to extract tarball: %w\nstdout: %s\nstderr: %s",
				output.Output.Err, string(output.Output.Stdout), string(output.Output.Stderr))
		}

		// Run asinstall
		installCmd = []string{"/bin/bash", "-c", fmt.Sprintf("export DEBIAN_FRONTEND=noninteractive; cd /tmp/%s/aerospike* && ./asinstall", ntime)}
	default:
		return errors.New("unknown upgrade type")
	}

	// Execute install command
	var stdout, stderr *os.File
	terminal := false
	if system.logLevel >= 5 {
		stdout = os.Stdout
		stderr = os.Stderr
		terminal = true
	}
	detail := sshexec.ExecDetail{
		Command:        installCmd,
		Terminal:       terminal,
		SessionTimeout: 15 * time.Minute,
	}
	if stdout != nil {
		detail.Stdout = stdout
	}
	if stderr != nil {
		detail.Stderr = stderr
	}
	output := instance.Exec(&backends.ExecInput{
		ExecDetail:      detail,
		Username:        "root",
		ConnectTimeout:  30 * time.Second,
		ParallelThreads: 1,
	})
	if output.Output.Err != nil {
		return fmt.Errorf("install command failed: %w\nstdout: %s\nstderr: %s",
			output.Output.Err, string(output.Output.Stdout), string(output.Output.Stderr))
	}

	// Restore aerospike.conf
	err = client.WriteFile(false, &sshexec.FileWriter{
		DestPath:    "/etc/aerospike/aerospike.conf",
		Source:      bytes.NewReader(confBackup.Bytes()),
		Permissions: 0644,
	})
	if err != nil {
		return fmt.Errorf("failed to restore aerospike.conf: %w", err)
	}

	// Start aerospike if restart is requested
	if c.RestartAerospike {
		logger.Info("Starting aerospike on %s:%d", instance.ClusterName, instance.NodeNo)
		output := instance.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"systemctl", "start", "aerospike"},
				SessionTimeout: 2 * time.Minute,
			},
			Username:        "root",
			ConnectTimeout:  30 * time.Second,
			ParallelThreads: 1,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to start aerospike: %w\nstdout: %s\nstderr: %s",
				output.Output.Err, string(output.Output.Stdout), string(output.Output.Stderr))
		}
	}

	logger.Info("Successfully upgraded aerospike on %s:%d", instance.ClusterName, instance.NodeNo)
	return nil
}
