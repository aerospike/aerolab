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
	"github.com/aerospike/aerolab/pkg/utils/scriptlog"
	"github.com/rglonek/logger"
)

type ClientCreateBaseCmd struct {
	ClientCreateNoneCmd

	// skipBaseInstall tells createBaseClient to skip running the base-tools
	// install script (curl/wget/vim/git/jq/unzip/zip). Callers set this when
	// those tools are already baked into the template image being used, to
	// avoid re-running apt/yum on every `client create ...` invocation.
	skipBaseInstall bool
}

// baseInstallSoftware returns the installers.Software spec describing the
// base tools that `client create base` (and every downstream client) relies
// on. It's exposed as a helper so template-build flows can bake the same
// tools into the template image and then tell createBaseClient to skip the
// per-instance install step.
func baseInstallSoftware(debug bool) installers.Software {
	return installers.Software{
		Debug: debug,
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
	}
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

	if c.skipBaseInstall {
		logger.Debug("Skipping base tools install (pre-installed in template image)")
		return clients, nil
	}

	logger.Info("Installing base tools on client instances")

	installScript, err := installers.GetInstallScript(baseInstallSoftware(system.logLevel >= 5), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to generate install script: %w", err)
	}

	// Upload and run install script on each client
	for _, client := range clients.Describe() {
		conf, err := client.GetSftpConfig("root")
		if err != nil {
			return nil, fmt.Errorf("failed to get SFTP config for %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}
		conf.MaxRetries = c.MaxRetries
		conf.RetrySleep = c.RetrySleep

		sftpClient, err := sshexec.NewSftp(conf)
		if err != nil {
			return nil, fmt.Errorf("failed to create SFTP client for %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}

		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/opt/aerolab/scripts/install-base.sh",
			Source:      strings.NewReader(string(installScript)),
			Permissions: 0755,
		})
		sftpClient.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to upload install script to %s:%d: %w", client.ClusterName, client.NodeNo, err)
		}

		// If debug level is selected, output to stdout/stderr
		var stdout, stderr *os.File
		var stdin io.ReadCloser
		terminal := false
		if system.logLevel >= 5 {
			stdout = os.Stdout
			stderr = os.Stderr
			stdin = io.NopCloser(os.Stdin)
			terminal = true
		}
		scriptPath := "/opt/aerolab/scripts/install-base.sh"
		execDetail := sshexec.ExecDetail{
			Command:        []string{"bash", scriptPath},
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
			MaxRetries:     c.MaxRetries,
			RetrySleep:     c.RetrySleep,
		})

		if output.Output.Err != nil {
			// Save script failure to local machine for debugging
			failure := scriptlog.NewScriptFailureWithPath(
				client.ClusterName,
				client.NodeNo,
				scriptPath,
				installScript,
				output.Output.Stdout,
				output.Output.Stderr,
				output.Output.Err,
			)
			logPath, saveErr := scriptlog.SaveFailure(failure)
			if saveErr != nil {
				return nil, fmt.Errorf("failed to run install script on %s:%d: %w (also failed to save logs: %v)", client.ClusterName, client.NodeNo, output.Output.Err, saveErr)
			}
			return nil, fmt.Errorf("%s", scriptlog.FormatError(logPath, client.ClusterName, client.NodeNo, output.Output.Err))
		}
	}

	return clients, nil
}
