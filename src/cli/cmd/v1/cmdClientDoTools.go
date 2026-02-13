package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike"
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
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

	defer UpdateDiskCache(system)()
	err = c.createToolsClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateToolsCmd) createToolsClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "create", "tools"}, c)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	// Override type
	if c.TypeOverride == "" {
		c.TypeOverride = "tools"
	}

	// Create base client first
	baseCmd := &ClientCreateBaseCmd{ClientCreateNoneCmd: c.ClientCreateNoneCmd}
	clients, err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return err
	}

	// Install aerospike tools
	logger.Info("Installing Aerospike tools version %s", c.ToolsVersion)

	for _, client := range clients.Describe() {
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
			return fmt.Errorf("failed to get Aerospike products for %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}

		product := products.WithName("aerospike-tools")
		if len(product) == 0 {
			return fmt.Errorf("no aerospike-tools product found for %s:%d", client.ClusterName, client.NodeNo)
		}

		versions, err := aerospike.GetVersions(30*time.Second, product[0])
		if err != nil {
			return fmt.Errorf("failed to get tools versions for %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}

		// Filter by version if specified
		if c.ToolsVersion.String() != "" && c.ToolsVersion.String() != "latest" {
			versions = versions.WithNamePrefix(c.ToolsVersion.String())
		}

		version := versions.Latest()
		if version == nil {
			return fmt.Errorf("no matching tools version found for %s:%d", client.ClusterName, client.NodeNo)
		}

		files, err := aerospike.GetFiles(30*time.Second, *version)
		if err != nil {
			return fmt.Errorf("failed to get tools files for %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}

		// Get tools installer script
		toolsInstaller, err := files.GetInstallScript(
			arch,
			aerospike.OSName(client.OperatingSystem.Name),
			client.OperatingSystem.Version,
			system.logLevel >= 5, // debug
			true,                 // download
			true,                 // install
			true,                 // upgrade
		)
		if err != nil {
			return fmt.Errorf("failed to get tools installer for %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}

		// Upload installer script
		conf, err := client.GetSftpConfig("root")
		if err != nil {
			return fmt.Errorf("failed to get SFTP config for %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}
		conf.MaxRetries = c.MaxRetries
		conf.RetrySleep = c.RetrySleep

		sftpClient, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("failed to create SFTP client for %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}

		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/opt/aerolab/scripts/install-tools.sh",
			Source:      strings.NewReader(string(toolsInstaller)),
			Permissions: 0755,
		})
		sftpClient.Close()
		if err != nil {
			return fmt.Errorf("failed to upload tools installer to %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}

		// Execute installer
		scriptPath := "/opt/aerolab/scripts/install-tools.sh"
		execDetail := sshexec.ExecDetail{
			Command:        []string{"bash", scriptPath},
			SessionTimeout: 15 * time.Minute,
		}
		if system.logLevel >= 5 {
			execDetail.Stdin = io.NopCloser(os.Stdin)
			execDetail.Stdout = os.Stdout
			execDetail.Stderr = os.Stderr
			execDetail.Terminal = true
		}
		output := client.Exec(&backends.ExecInput{
			ExecDetail:     execDetail,
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
			MaxRetries:     c.MaxRetries,
			RetrySleep:     c.RetrySleep,
		})

		if output.Output.Err != nil {
			// Save script failure to local machine for debugging
			failure := scriptlog.NewScriptFailureWithPath(
				client.ClusterName,
				client.NodeNo,
				scriptPath,
				toolsInstaller,
				output.Output.Stdout,
				output.Output.Stderr,
				output.Output.Err,
			)
			logPath, saveErr := scriptlog.SaveFailure(failure)
			if saveErr != nil {
				return fmt.Errorf("failed to install tools on %s:%d: %w (also failed to save logs: %v)", client.ClusterName, client.NodeNo, output.Output.Err, saveErr)
			}
			return fmt.Errorf("%s", scriptlog.FormatError(logPath, client.ClusterName, client.NodeNo, output.Output.Err))
		}
		logger.Info("Successfully installed Aerospike tools on %s:%d", client.ClusterName, client.NodeNo)
	}

	return nil
}
