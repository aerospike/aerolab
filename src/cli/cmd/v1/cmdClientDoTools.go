package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike"
	"github.com/rglonek/logger"
)

type ClientCreateToolsCmd struct {
	ClientCreateNoneCmd
	ToolsVersion TypeAerospikeVersion `short:"v" long:"tools-version" description:"Aerospike tools version to install" default:"latest"`
}

func (c *ClientCreateToolsCmd) Execute(args []string) error {
	isGrow := len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow"

	var cmd []string
	if isGrow {
		cmd = []string{"client", "grow", "tools"}
	} else {
		cmd = []string{"client", "create", "tools"}
	}

	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.createToolsClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateToolsCmd) createToolsClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) error {
	// Override type
	if c.TypeOverride == "" {
		c.TypeOverride = "tools"
	}

	// Create base client first
	baseCmd := &ClientCreateBaseCmd{ClientCreateNoneCmd: c.ClientCreateNoneCmd}
	err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return err
	}

	// Install aerospike tools
	logger.Info("Installing Aerospike tools version %s", c.ToolsVersion)

	// Get created instances
	clients := system.Backend.GetInventory().Instances.
		WithTags(map[string]string{"aerolab.old.type": "client"}).
		WithClusterName(c.ClientName.String()).
		WithState(backends.LifeCycleStateRunning)

	if clients.Count() == 0 {
		return fmt.Errorf("no running client instances found after creation")
	}

	clientList := clients.Describe()
	for _, client := range clientList {
		// Determine architecture
		var arch aerospike.ArchitectureType
		if client.Architecture == backends.ArchitectureARM64 {
			arch = aerospike.ArchitectureTypeAARCH64
		} else {
			arch = aerospike.ArchitectureTypeX86_64
		}

		// Get aerospike tools products and versions
		products, err := aerospike.GetProducts(30 * time.Second)
		if err != nil {
			logger.Warn("Failed to get Aerospike products for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		product := products.WithName("aerospike-tools")
		if len(product) == 0 {
			logger.Warn("No aerospike-tools product found for %s:%d", client.ClusterName, client.NodeNo)
			continue
		}

		versions, err := aerospike.GetVersions(30*time.Second, product[0])
		if err != nil {
			logger.Warn("Failed to get tools versions for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		// Filter by version if specified
		if c.ToolsVersion.String() != "" && c.ToolsVersion.String() != "latest" {
			versions = versions.WithNamePrefix(c.ToolsVersion.String())
		}

		version := versions.Latest()
		if version == nil {
			logger.Warn("No matching tools version found for %s:%d", client.ClusterName, client.NodeNo)
			continue
		}

		files, err := aerospike.GetFiles(30*time.Second, *version)
		if err != nil {
			logger.Warn("Failed to get tools files for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		// Get tools installer script
		toolsInstaller, err := files.GetInstallScript(
			arch,
			aerospike.OSName(client.OperatingSystem.Name),
			client.OperatingSystem.Version,
			true,  // debug
			true,  // download
			true,  // install
			false, // upgrade
		)
		if err != nil {
			logger.Warn("Failed to get tools installer for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		// Upload installer script
		conf, err := client.GetSftpConfig("root")
		if err != nil {
			logger.Warn("Failed to get SFTP config for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		sftpClient, err := sshexec.NewSftp(conf)
		if err != nil {
			logger.Warn("Failed to create SFTP client for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/tmp/install-tools.sh",
			Source:      strings.NewReader(string(toolsInstaller)),
			Permissions: 0755,
		})
		sftpClient.Close()
		if err != nil {
			logger.Warn("Failed to upload tools installer to %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		// Execute installer
		output := client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "/tmp/install-tools.sh"},
				SessionTimeout: 15 * time.Minute,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})

		if output.Output.Err != nil {
			logger.Warn("Failed to install tools on %s:%d: %s", client.ClusterName, client.NodeNo, output.Output.Err)
		} else {
			logger.Info("Successfully installed Aerospike tools on %s:%d", client.ClusterName, client.NodeNo)
		}
	}

	return nil
}
