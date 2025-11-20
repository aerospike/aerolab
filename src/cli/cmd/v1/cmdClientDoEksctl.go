package cmd

import (
	"fmt"
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

	err = c.createEksCtlClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateEksCtlCmd) createEksCtlClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) error {
	// Override type
	if c.TypeOverride == "" {
		c.TypeOverride = "eksctl"
	}

	// Create base client first
	baseCmd := &ClientCreateBaseCmd{ClientCreateNoneCmd: c.ClientCreateNoneCmd}
	err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return err
	}

	// Install eksctl and kubectl
	logger.Info("Installing eksctl and kubectl")
	
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

