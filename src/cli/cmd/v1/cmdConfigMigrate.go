package cmd

import (
	"fmt"
	"os"
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
	return nil
}
