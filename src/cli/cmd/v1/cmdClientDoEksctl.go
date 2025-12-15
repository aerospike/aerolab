package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/eks"
	"github.com/aerospike/aerolab/pkg/sshexec"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerolab"
	"github.com/aerospike/aerolab/pkg/utils/installers/eksctl"
	"github.com/aerospike/aerolab/pkg/utils/parallelize"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ClientCreateEksCtlCmd struct {
	ClientCreateNoneCmd
	EksAwsRegion          string         `short:"r" long:"eks-aws-region" description:"AWS region to install expiries system too and configure as default region"`
	EksAwsKeyId           string         `short:"k" long:"eks-aws-keyid" description:"AWS Key ID to use for auth when performing eksctl tasks and expiries"`
	EksAwsSecretKey       string         `short:"s" long:"eks-aws-secretkey" description:"AWS Secret Key to use for auth when performing eksctl tasks and expiries"`
	EksAwsInstanceProfile string         `short:"x" long:"eks-aws-profile" description:"AWS instance profile to use instead of KEYID/SecretKey for authentication"`
	FeaturesFilePath      flags.Filename `short:"f" long:"eks-asd-features" description:"Aerospike Features File to copy to the EKSCTL client machine; destination: /root/features.conf"`
}

func (c *ClientCreateEksCtlCmd) Execute(args []string) error {
	isGrow := len(os.Args) >= 3 && os.Args[1] == "client" && os.Args[2] == "grow"

	var cmd []string
	if isGrow {
		cmd = []string{"client", "grow", "eksctl"}
	} else {
		cmd = []string{"client", "create", "eksctl"}
	}

	// Validation
	if c.Version == "latest" {
		c.Version = "24.04"
	}
	if c.OS != "ubuntu" || c.Version != "24.04" {
		return fmt.Errorf("eksctl is only supported on ubuntu:24.04, selected %s:%s", c.OS, c.Version)
	}

	// Handle ENV:: prefix for credentials
	if strings.HasPrefix(c.EksAwsKeyId, "ENV::") {
		c.EksAwsKeyId = os.Getenv(strings.Split(c.EksAwsKeyId, "::")[1])
	}
	if strings.HasPrefix(c.EksAwsSecretKey, "ENV::") {
		c.EksAwsSecretKey = os.Getenv(strings.Split(c.EksAwsSecretKey, "::")[1])
	}

	// Validate required parameters
	if c.FeaturesFilePath == "" {
		return errors.New("features file must be specified using -f /path/to/features.conf")
	}
	if (c.EksAwsKeyId == "" || c.EksAwsSecretKey == "") && c.EksAwsInstanceProfile == "" {
		return errors.New("either KeyID+SecretKey OR InstanceProfile must be specified; for help see: aerolab client create eksctl help")
	}
	if c.EksAwsRegion == "" {
		return errors.New("AWS region must be specified (use -r AWSREGION)")
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
	system.Logger.Info("To configure timezone inside the machine, run: aerolab attach client -n %s -- dpkg-reconfigure tzdata", c.ClientName)
	system.Logger.Info("Attach command: aerolab attach client -n %s", c.ClientName)
	system.Logger.Info("Usage instructions: https://github.com/aerospike/aerolab/blob/master/docs/eks/README.md")
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

	// Read features file
	features, err := os.ReadFile(string(c.FeaturesFilePath))
	if err != nil {
		return fmt.Errorf("could not read features file: %w", err)
	}

	// Override type and set instance profile if specified
	if c.TypeOverride == "" {
		c.TypeOverride = "eksctl"
	}
	if c.EksAwsInstanceProfile != "" {
		// Set instance profile for base client creation
		c.ClientCreateNoneCmd.AWS.IAMInstanceProfile = c.EksAwsInstanceProfile
	}

	// Create base client first
	baseCmd := &ClientCreateBaseCmd{ClientCreateNoneCmd: c.ClientCreateNoneCmd}
	clients, err := baseCmd.createBaseClient(system, inventory, logger, args, isGrow)
	if err != nil {
		return err
	}

	if clients.Count() == 0 {
		return errors.New("no clients were created")
	}

	logger.Info("Continuing eksctl installation...")

	// Get installation scripts
	eksctlScript, err := eksctl.GetInstallScript()
	if err != nil {
		return fmt.Errorf("failed to get eksctl installer: %w", err)
	}

	bootstrapScript, err := eksctl.GetBootstrapScript()
	if err != nil {
		return fmt.Errorf("failed to get bootstrap script: %w", err)
	}

	aerolabScript, err := aerolab.GetLinuxInstallScript(nil, nil)
	if err != nil {
		return fmt.Errorf("failed to get aerolab installer: %w", err)
	}

	// Get EKS YAML templates
	eksYamlFiles, err := eks.GetTemplates(c.EksAwsRegion)
	if err != nil {
		return fmt.Errorf("failed to get EKS YAML templates: %w", err)
	}

	// Process each client in parallel
	returns := parallelize.MapLimit(clients.Describe(), c.ParallelSSHThreads, func(client *backends.Instance) error {
		logger.Info("Configuring %s:%d", client.ClusterName, client.NodeNo)

		// Get SFTP client
		conf, err := client.GetSftpConfig("root")
		if err != nil {
			return fmt.Errorf("failed to get SFTP config: %w", err)
		}

		sftpClient, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("failed to create SFTP client: %w", err)
		}
		defer sftpClient.Close()

		// 1. Upload and run eksctl installer
		logger.Debug("Uploading eksctl installer to %s:%d", client.ClusterName, client.NodeNo)
		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/tmp/install-eksctl.sh",
			Source:      bytes.NewReader(eksctlScript),
			Permissions: 0755,
		})
		if err != nil {
			return fmt.Errorf("failed to upload eksctl installer: %w", err)
		}

		logger.Debug("Installing eksctl on %s:%d", client.ClusterName, client.NodeNo)
		output := client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "/tmp/install-eksctl.sh"},
				SessionTimeout: 15 * time.Minute,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to install eksctl: %w", output.Output.Err)
		}

		// 2. Upload bootstrap script
		logger.Debug("Uploading bootstrap script to %s:%d", client.ClusterName, client.NodeNo)
		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/usr/local/bin/bootstrap",
			Source:      bytes.NewReader(bootstrapScript),
			Permissions: 0755,
		})
		if err != nil {
			return fmt.Errorf("failed to upload bootstrap script: %w", err)
		}

		// 3. Run bootstrap script with parameters
		bootstrapCmd := []string{"/bin/bash", "/usr/local/bin/bootstrap", "-r", c.EksAwsRegion}
		if c.EksAwsKeyId != "" && c.EksAwsSecretKey != "" {
			bootstrapCmd = append(bootstrapCmd, "-k", c.EksAwsKeyId, "-s", c.EksAwsSecretKey)
		}

		logger.Debug("Running bootstrap script on %s:%d", client.ClusterName, client.NodeNo)
		output = client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        bootstrapCmd,
				SessionTimeout: 15 * time.Minute,
				Stdout:         os.Stdout,
				Stderr:         os.Stderr,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to run bootstrap script: %w", output.Output.Err)
		}

		// 4. Upload and install aerolab
		logger.Debug("Uploading aerolab installer to %s:%d", client.ClusterName, client.NodeNo)
		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/tmp/install-aerolab.sh",
			Source:      bytes.NewReader(aerolabScript),
			Permissions: 0755,
		})
		if err != nil {
			return fmt.Errorf("failed to upload aerolab installer: %w", err)
		}

		logger.Debug("Installing aerolab on %s:%d", client.ClusterName, client.NodeNo)
		output = client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"bash", "/tmp/install-aerolab.sh"},
				SessionTimeout: 15 * time.Minute,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to install aerolab: %w", output.Output.Err)
		}

		// 5. Create eksexpiry symlink
		logger.Debug("Creating eksexpiry symlink on %s:%d", client.ClusterName, client.NodeNo)
		output = client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"ln", "-sf", "/usr/local/bin/aerolab", "/usr/local/bin/eksexpiry"},
				SessionTimeout: 30 * time.Second,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to create eksexpiry symlink: %w", output.Output.Err)
		}

		// 7. Create /root/eks directory
		logger.Debug("Creating /root/eks directory on %s:%d", client.ClusterName, client.NodeNo)
		output = client.Exec(&backends.ExecInput{
			ExecDetail: sshexec.ExecDetail{
				Command:        []string{"mkdir", "-p", "/root/eks"},
				SessionTimeout: 30 * time.Second,
			},
			Username:       "root",
			ConnectTimeout: 30 * time.Second,
		})
		if output.Output.Err != nil {
			return fmt.Errorf("failed to create /root/eks directory: %w", output.Output.Err)
		}

		// 8. Upload EKS YAML templates
		logger.Debug("Uploading EKS YAML templates to %s:%d", client.ClusterName, client.NodeNo)
		for filename, contents := range eksYamlFiles {
			err = sftpClient.WriteFile(true, &sshexec.FileWriter{
				DestPath:    path.Join("/root/eks", filename),
				Source:      bytes.NewReader(contents),
				Permissions: 0644,
			})
			if err != nil {
				return fmt.Errorf("failed to upload %s: %w", filename, err)
			}
		}

		// 9. Upload features file
		logger.Debug("Uploading features file to %s:%d", client.ClusterName, client.NodeNo)
		err = sftpClient.WriteFile(true, &sshexec.FileWriter{
			DestPath:    "/root/features.conf",
			Source:      bytes.NewReader(features),
			Permissions: 0644,
		})
		if err != nil {
			return fmt.Errorf("failed to upload features file: %w", err)
		}

		logger.Info("Successfully configured %s:%d", client.ClusterName, client.NodeNo)
		return nil
	})

	// Check for errors
	var hasError bool
	for i, ret := range returns {
		if ret != nil {
			logger.Error("Node %d returned: %s", i+1, ret)
			hasError = true
		}
	}

	if hasError {
		return errors.New("some nodes returned errors")
	}

	return nil
}
