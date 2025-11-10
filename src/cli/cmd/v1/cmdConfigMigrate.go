package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TODO: FUTURE: ADD OPTION TO MIGRATE RESOURCES TOO (VMs, etc) TOGETHER WITH SSH KEYS
type ConfigMigrateCmd struct {
	OldDir string  `short:"o" long:"old-dir" description:"Old AeroLab directory to migrate from"`
	NewDir string  `short:"n" long:"new-dir" description:"New AeroLab directory to migrate to"`
	Help   HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigMigrateCmd) Execute(args []string) error {
	cmd := []string{"config", "migrate"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	if c.NewDir == "" {
		newDir, err := AerolabRootDir()
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
		c.NewDir = newDir
	}
	if c.OldDir == "" {
		oldDir, err := AerolabRootDirOld()
		if err != nil {
			return Error(err, system, cmd, c, args)
		}
		c.OldDir = oldDir
	}
	err = MigrateAerolabConfig(c.OldDir, c.NewDir)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func MigrateAerolabConfig(oldDir string, newDir string) error {
	os.MkdirAll(newDir, 0700)
	// copy oldDir/conf and oldDir/conf.ts to newDir/
	oldConf := filepath.Join(oldDir, "conf")
	oldConfTs := filepath.Join(oldDir, "conf.ts")
	newConf := filepath.Join(newDir, "conf")
	newConfTs := filepath.Join(newDir, "conf.ts")

	// Copy conf file if it exists
	if _, err := os.Stat(oldConf); err == nil {
		data, err := os.ReadFile(oldConf)
		if err != nil {
			return fmt.Errorf("failed to read old conf file: %w", err)
		}
		err = os.WriteFile(newConf, data, 0600)
		if err != nil {
			return fmt.Errorf("failed to write new conf file: %w", err)
		}
	}

	// Copy conf.ts file if it exists
	if _, err := os.Stat(oldConfTs); err == nil {
		data, err := os.ReadFile(oldConfTs)
		if err != nil {
			return fmt.Errorf("failed to read old conf.ts file: %w", err)
		}
		err = os.WriteFile(newConfTs, data, 0600)
		if err != nil {
			return fmt.Errorf("failed to write new conf.ts file: %w", err)
		}
	}

	// Fix docker backend region if needed
	err := fixDockerBackendRegion(newConf)
	if err != nil {
		return fmt.Errorf("failed to fix docker backend region: %w", err)
	}

	return nil
}

// fixDockerBackendRegion checks if backend type is docker and resets region to empty
func fixDockerBackendRegion(confFile string) error {
	// Check if config file exists
	if _, err := os.Stat(confFile); os.IsNotExist(err) {
		return nil // No config file, nothing to fix
	}

	// Get the path to self
	selfPath, err := GetSelfPath()
	if err != nil {
		return fmt.Errorf("failed to get self path: %w", err)
	}

	// Check backend type using config defaults
	cmd := exec.Command(selfPath, "config", "defaults", "-k", "Config.Backend.Type")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check backend type: %w: %s", err, string(output))
	}

	// Parse output: "Config.Backend.Type = docker"
	outputStr := strings.TrimSpace(string(output))
	if !strings.Contains(outputStr, "= docker") {
		return nil // Not docker, nothing to fix
	}

	// Check if region is set
	cmd = exec.Command(selfPath, "config", "defaults", "-k", "Config.Backend.Region")
	output, err = cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to check backend region: %w: %s", err, string(output))
	}

	// Parse output: "Config.Backend.Region = someregion" or "Config.Backend.Region = "
	outputStr = strings.TrimSpace(string(output))
	parts := strings.SplitN(outputStr, " = ", 2)
	if len(parts) != 2 {
		return nil // Can't parse, skip
	}
	region := strings.TrimSpace(parts[1])

	// If region is not empty, reset it by calling config backend
	if region != "" {
		cmd = exec.Command(selfPath, "config", "backend", "-t", "docker")
		output, err = cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to reset backend region: %w: %s", err, string(output))
		}
	}

	return nil
}
