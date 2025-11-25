package cmd

import (
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/eksctl"
	"github.com/rglonek/logger"
)

type ClientCreateEksCtlCmd struct {
	ClientCreateNoneCmd
}

func (c *ClientCreateEksCtlCmd) Execute(args []string) error {
	isGrow := len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow"

	var cmd []string
	if isGrow {
		cmd = []string{"client", "grow", "eksctl"}
	} else {
		cmd = []string{"client", "create", "eksctl"}
	}

	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.createEksCtlClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateEksCtlCmd) createEksCtlClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "create", "eksctl"}, c)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	// Override type
	if c.TypeOverride == "" {
		c.TypeOverride = "eksctl"
	}

	// Create base client first
	baseCmd := &ClientCreateBaseCmd{ClientCreateNoneCmd: c.ClientCreateNoneCmd}
	clients, err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return err
	}

	// Install eksctl and kubectl
	logger.Info("Installing eksctl and kubectl")

	for _, client := range clients.Describe() {
		// Get eksctl installer
		eksctlScript, err := eksctl.GetInstallScript()
		if err != nil {
			logger.Warn("Failed to get eksctl installer for %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

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
			DestPath:    "/tmp/install-eksctl.sh",
			Source:      strings.NewReader(string(eksctlScript)),
			Permissions: 0755,
		})
		sftpClient.Close()
		if err != nil {
			logger.Warn("Failed to upload eksctl installer to %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		// Execute installer
		output := client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "/tmp/install-eksctl.sh"},
				SessionTimeout: 15 * time.Minute,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})

		if output.Output.Err != nil {
			logger.Warn("Failed to install eksctl on %s:%d: %s", client.ClusterName, client.NodeNo, output.Output.Err)
		} else {
			logger.Info("Successfully installed eksctl on %s:%d", client.ClusterName, client.NodeNo)
		}
	}

	return nil
}
