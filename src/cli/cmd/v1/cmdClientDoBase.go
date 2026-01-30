package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers"
	"github.com/rglonek/logger"
)

type ClientCreateBaseCmd struct {
	ClientCreateNoneCmd
}

func (c *ClientCreateBaseCmd) Execute(args []string) error {
	isGrow := len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow"

	var cmd []string
	if isGrow {
		cmd = []string{"client", "grow", "base"}
	} else {
		cmd = []string{"client", "create", "base"}
	}

	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	_, err = c.createBaseClient(system, system.Backend.GetInventory(), system.Logger, args, isGrow)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ClientCreateBaseCmd) createBaseClient(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string, isGrow bool) (backends.InstanceList, error) {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"client", "create", "base"}, c)
		if err != nil {
			return nil, err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}
	// Override type
	if c.TypeOverride == "" {
		c.TypeOverride = "base"
	}

	// Create base client
	clients, err := c.createNoneClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return nil, err
	}

	logger.Info("Installing base tools on client instances")

	// Install basic dependencies
	installScript, err := installers.GetInstallScript(installers.Software{
		Debug: system.logLevel >= 5,
		Optional: installers.Installs{
			Dependencies: []installers.Dependency{
				{Command: "curl", Package: "curl"},
				{Command: "wget", Package: "wget"},
				{Command: "vim", Package: "vim"},
				{Command: "git", Package: "git"},
				{Command: "jq", Package: "jq"},
				{Command: "unzip", Package: "unzip"},
				{Command: "zip", Package: "zip"},
			},
		},
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate install script: %w", err)
	}

	// Upload and run install script on each client
	for _, client := range clients.Describe() {
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
			DestPath:    "/tmp/install-base.sh",
			Source:      strings.NewReader(string(installScript)),
			Permissions: 0755,
		})
		sftpClient.Close()
		if err != nil {
			logger.Warn("Failed to upload install script to %s:%d: %s", client.ClusterName, client.NodeNo, err)
			continue
		}

		// If debug level is selected, output to stdout/stderr and enable terminal mode
		var stdout, stderr *os.File
		var stdin io.ReadCloser
		terminal := false
		if system.logLevel >= 5 {
			stdout = os.Stdout
			stderr = os.Stderr
			stdin = io.NopCloser(os.Stdin)
			terminal = true
		}
		execDetail := sshexec.ExecDetail{
			Command:        []string{"bash", "/tmp/install-base.sh"},
			SessionTimeout: 30 * time.Minute,
			Terminal:       terminal,
		}
		if system.logLevel >= 5 {
			execDetail.Stdin = stdin
			execDetail.Stdout = stdout
			execDetail.Stderr = stderr
		}
		// Execute install script
		output := client.Exec(&backends.ExecInput{
			ExecDetail:     execDetail,
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})

		if output.Output.Err != nil {
			logger.Warn("Failed to run install script on %s:%d: %s", client.ClusterName, client.NodeNo, output.Output.Err)
			logger.Warn("stdout: %s", output.Output.Stdout)
			logger.Warn("stderr: %s", output.Output.Stderr)
		}
	}

	return clients, nil
}
