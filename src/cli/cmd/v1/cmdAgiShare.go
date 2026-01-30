package cmd

import (
	"bytes"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/sshexec"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

// AgiShareCmd shares an AGI instance by adding SSH public keys to authorized_keys.
// This allows other users to SSH into the AGI instance for administration.
type AgiShareCmd struct {
	ClusterName     TypeAgiClusterName `short:"n" long:"name" description:"AGI name" default:"agi"`
	KeyFile         flags.Filename  `short:"f" long:"pubkey" description:"Path to a pubkey to import to AGI instance"`
	Key             string          `short:"k" long:"key" description:"SSH public key content to add (alternative to --pubkey)"`
	Remove          bool            `short:"r" long:"remove" description:"Remove the specified key instead of adding it"`
	List            bool            `short:"l" long:"list" description:"List all authorized keys on the AGI instance"`
	ParallelThreads int             `short:"p" long:"parallel-threads" description:"Number of parallel threads to use" default:"10"`
	ConnectTimeout  time.Duration   `short:"C" long:"connect-timeout" description:"Connect timeout" default:"10s"`
	Help            HelpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

// Execute implements the command execution for agi share.
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiShareCmd) Execute(args []string) error {
	cmd := []string{"agi", "share"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.Share(system, system.Backend.GetInventory(), system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// Share manages SSH keys for the AGI instance.
//
// Parameters:
//   - system: The initialized system context
//   - inventory: The current backend inventory
//   - logger: Logger for output
//   - args: Additional command arguments
//
// Returns:
//   - error: nil on success, or an error describing what failed
func (c *AgiShareCmd) Share(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: true, ExistingInventory: inventory}, []string{"agi", "share"}, c, args...)
		if err != nil {
			return err
		}
	}
	if inventory == nil {
		inventory = system.Backend.GetInventory()
	}

	// Get AGI instance
	instance := inventory.Instances.WithClusterName(string(c.ClusterName)).WithState(backends.LifeCycleStateRunning)
	if instance.Count() == 0 {
		return fmt.Errorf("AGI instance %s not found or not running", c.ClusterName)
	}
	inst := instance.Describe()[0]

	// Handle list operation
	if c.List {
		return c.listKeys(inst, logger)
	}

	// Determine the key to add/remove
	var pubkey []byte
	var err error

	if c.KeyFile != "" {
		pubkey, err = os.ReadFile(string(c.KeyFile))
		if err != nil {
			return fmt.Errorf("failed to read key file %s: %w", string(c.KeyFile), err)
		}
	} else if c.Key != "" {
		pubkey = []byte(c.Key)
	} else {
		return fmt.Errorf("either --pubkey or --key must be specified")
	}

	// Validate key format
	keyStr := strings.TrimSpace(string(pubkey))
	if !strings.HasPrefix(keyStr, "ssh-") && !strings.HasPrefix(keyStr, "ecdsa-") {
		return fmt.Errorf("key does not look like an SSH public key (should start with ssh- or ecdsa-)")
	}

	// Handle remove operation
	if c.Remove {
		return c.removeKey(inst, keyStr, logger)
	}

	// Add the key
	return c.addKey(inst, keyStr, logger)
}

// listKeys lists all authorized keys on the AGI instance.
func (c *AgiShareCmd) listKeys(inst *backends.Instance, logger *logger.Logger) error {
	outputs := backends.InstanceList{inst}.Exec(&backends.ExecInput{
		ExecDetail: sshexec.ExecDetail{
			Command:        []string{"cat", "/root/.ssh/authorized_keys"},
			SessionTimeout: time.Minute,
		},
		Username:        "root",
		ConnectTimeout:  c.ConnectTimeout,
		ParallelThreads: 1,
	})

	if len(outputs) > 0 {
		if outputs[0].Output.Err != nil {
			return fmt.Errorf("failed to list keys: %w", outputs[0].Output.Err)
		}
		fmt.Println(string(outputs[0].Output.Stdout))
	}

	return nil
}

// addKey adds an SSH public key to the authorized_keys file.
func (c *AgiShareCmd) addKey(inst *backends.Instance, key string, logger *logger.Logger) error {
	confs, err := backends.InstanceList{inst}.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	for _, conf := range confs {
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("failed to create SFTP client: %w", err)
		}
		defer cli.Close()

		// Ensure .ssh directory exists
		if !cli.IsExists("/root/.ssh") {
			err = cli.Mkdir("/root/.ssh", 0700)
			if err != nil {
				return fmt.Errorf("failed to create .ssh directory: %w", err)
			}
		}

		// Read existing authorized_keys
		var authorizedKeys []byte
		if cli.IsExists("/root/.ssh/authorized_keys") {
			var buf bytes.Buffer
			fr := sshexec.FileReader{
				SourcePath:  "/root/.ssh/authorized_keys",
				Destination: &buf,
			}
			err = cli.ReadFile(&fr)
			if err != nil {
				return fmt.Errorf("failed to read authorized_keys: %w", err)
			}
			authorizedKeys = buf.Bytes()

			// Check if key already exists
			if strings.Contains(string(authorizedKeys), key) {
				logger.Info("Key already exists in authorized_keys")
				return nil
			}

			// Ensure trailing newline
			if len(authorizedKeys) > 0 && authorizedKeys[len(authorizedKeys)-1] != '\n' {
				authorizedKeys = append(authorizedKeys, '\n')
			}
		}

		// Append new key
		authorizedKeys = append(authorizedKeys, []byte(key)...)
		if !strings.HasSuffix(key, "\n") {
			authorizedKeys = append(authorizedKeys, '\n')
		}

		// Write updated authorized_keys
		fw := sshexec.FileWriter{
			DestPath:    "/root/.ssh/authorized_keys",
			Source:      bytes.NewReader(authorizedKeys),
			Permissions: 0600,
		}
		err = cli.WriteFile(false, &fw)
		if err != nil {
			return fmt.Errorf("failed to write authorized_keys: %w", err)
		}
	}

	logger.Info("SSH key added successfully")
	return nil
}

// removeKey removes an SSH public key from the authorized_keys file.
func (c *AgiShareCmd) removeKey(inst *backends.Instance, key string, logger *logger.Logger) error {
	confs, err := backends.InstanceList{inst}.GetSftpConfig("root")
	if err != nil {
		return fmt.Errorf("failed to get SFTP config: %w", err)
	}

	for _, conf := range confs {
		cli, err := sshexec.NewSftp(conf)
		if err != nil {
			return fmt.Errorf("failed to create SFTP client: %w", err)
		}
		defer cli.Close()

		// Read existing authorized_keys
		if !cli.IsExists("/root/.ssh/authorized_keys") {
			logger.Info("No authorized_keys file found")
			return nil
		}

		var buf bytes.Buffer
		fr := sshexec.FileReader{
			SourcePath:  "/root/.ssh/authorized_keys",
			Destination: &buf,
		}
		err = cli.ReadFile(&fr)
		if err != nil {
			return fmt.Errorf("failed to read authorized_keys: %w", err)
		}

		// Remove the key
		lines := strings.Split(buf.String(), "\n")
		var newLines []string
		found := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Match by the entire key or just the key fingerprint portion
			if strings.Contains(line, strings.TrimSpace(key)) || line == key {
				found = true
				continue
			}
			newLines = append(newLines, line)
		}

		if !found {
			logger.Warn("Key not found in authorized_keys")
			return nil
		}

		// Write updated authorized_keys
		newContent := strings.Join(newLines, "\n")
		if len(newContent) > 0 {
			newContent += "\n"
		}

		fw := sshexec.FileWriter{
			DestPath:    "/root/.ssh/authorized_keys",
			Source:      bytes.NewReader([]byte(newContent)),
			Permissions: 0600,
		}
		err = cli.WriteFile(false, &fw)
		if err != nil {
			return fmt.Errorf("failed to write authorized_keys: %w", err)
		}
	}

	logger.Info("SSH key removed successfully")
	return nil
}
