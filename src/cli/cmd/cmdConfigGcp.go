package cmd

import (
	"errors"
	"os"
	"path"
	"strings"
)

type ConfigGcpCmd struct {
	Reauthenticate   ReauthenticateCmd  `command:"reauthenticate" subcommands-optional:"true" description:"reauthenticate with GCP" webicon:"fas fa-key"`
	EnableServices   EnableServicesCmd  `command:"enable-services" subcommands-optional:"true" hidden:"true" webhidden:"true" description:"not needed anymore"` // NOTE: obsolete, but kept for backwards compatibility
	ListSecGroups    ListFirewallCmd    `command:"list-firewall-rules" subcommands-optional:"true" description:"list current aerolab-managed firewall rules" webicon:"fas fa-list"`
	CreateSecGroups  CreateFirewallCmd  `command:"create-firewall-rules" subcommands-optional:"true" description:"create AeroLab-managed firewall rules" webicon:"fas fa-circle-plus" invwebforce:"true"`
	LockSecGroups    LockFirewallCmd    `command:"lock-firewall-rules" subcommands-optional:"true" description:"lock the client firewall rules so that AMS/vscode are only accessible from a set IP" webicon:"fas fa-lock" invwebforce:"true"`
	DestroySecGroups DestroyFirewallCmd `command:"delete-firewall-rules" subcommands-optional:"true" description:"delete aerolab-managed firewall rules" webicon:"fas fa-trash" invwebforce:"true"`
	ExpiryInstall    ExpiryInstallCmd   `command:"expiry-install" subcommands-optional:"true" description:"install the expiry system scheduler and lambda with the required IAM roles" webicon:"fas fa-plus" invwebforce:"true"`
	ExpiryRemove     ExpiryRemoveCmd    `command:"expiry-remove" subcommands-optional:"true" description:"remove the expiry system scheduler, lambda and created IAM roles" webicon:"fas fa-minus" invwebforce:"true"`
	ExpiryCheckFreq  ExpiryCheckFreqCmd `command:"expiry-run-frequency" subcommands-optional:"true" description:"adjust how often the scheduler runs the expiry check lambda" webicon:"fas fa-gauge" invwebforce:"true"`
	ExpiryList       ExpiryListCmd      `command:"expiry-list" subcommands-optional:"true" description:"list the expiry systems" webicon:"fas fa-list"`
	Help             HelpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigGcpCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

type ReauthenticateCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ReauthenticateCmd) Execute(args []string) error {
	cmd := []string{"config", "gcp", "reauthenticate"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = c.Reauthenticate(system)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ReauthenticateCmd) Reauthenticate(system *System) error {
	if system.Opts.Config.Backend.Type != "gcp" {
		return errors.New("this command is only available for GCP backend type")
	}
	rootDir, err := AerolabRootDir()
	if err != nil {
		return err
	}
	tokenCacheFilePath := path.Join(rootDir, "default", "config", "gcp", "token-cache.json")
	if _, err := os.Stat(tokenCacheFilePath); err == nil {
		err = os.Remove(tokenCacheFilePath)
		if err != nil {
			return err
		}
	}
	err = system.GetBackend(false)
	if err != nil {
		return err
	}
	return nil
}

type EnableServicesCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *EnableServicesCmd) Execute(args []string) error {
	cmd := []string{"config", "gcp", "enable-services"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Function config.enable-services is obsolete and no longer required.")
	return Error(nil, system, cmd, c, args)
}

type ListFirewallCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type DestroyFirewallCmd struct {
	NamePrefix string  `short:"n" long:"name" description:"Name prefix to use for the firewall" default:"AeroLab"`
	All        bool    `short:"a" long:"all" description:"Remove all firewalls (ignores name parameter)"`
	VPC        string  `short:"v" long:"vpc" hidden:"true" webhidden:"true" description:"this no longer applies"` // NOTE: obsolete, but kept for backwards compatibility
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type CreateFirewallCmd struct {
	NamePrefix string   `short:"n" long:"name" description:"Name prefix to use for the firewall" default:"AeroLab"`
	IP         string   `short:"i" long:"ip" description:"set the IP mask to allow access, eg 0.0.0.0/0 or 1.2.3.4/32 or 10.11.12.13" default:"discover-caller-ip"`
	Ports      []string `short:"p" long:"port" description:"ports to open, can be specified multiple times, ex: 3000-3005 or tcp:3000-3005 or udp:3000"`
	NoDefaults bool     `short:"d" long:"no-defaults" hidden:"true" webhidden:"true" description:"this no longer applies"` // NOTE: obsolete, but kept for backwards compatibility
	VPC        string   `short:"v" long:"vpc" description:"vpc ID; default: use default VPC" default:""`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type LockFirewallCmd struct {
	NamePrefix string   `short:"n" long:"name" description:"Name prefix to use for the firewall" default:"AeroLab"`
	IP         string   `short:"i" long:"ip" description:"set the IP mask to allow access, eg 0.0.0.0/0 or 1.2.3.4/32 or 10.11.12.13" default:"discover-caller-ip"`
	VPC        string   `short:"v" long:"vpc" description:"VPC to handle sec groups for; default: default-VPC" default:""`
	Ports      []string `short:"p" long:"port" description:"ports to open, can be specified multiple times, ex: 3000-3005 or tcp:3000-3005 or udp:3000"`
	NoDefaults bool     `short:"d" long:"no-defaults" hidden:"true" webhidden:"true" description:"this no longer applies"` // NOTE: obsolete, but kept for backwards compatibility
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ListFirewallCmd) Execute(args []string) error {
	cmd := []string{"config", "gcp", "list-firewall-rules"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = ListSecurityGroups(system, c.Output, c.TableTheme, c.SortBy, "gcp")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *CreateFirewallCmd) Execute(args []string) error {
	cmd := []string{"config", "gcp", "create-firewall-rules"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	if c.IP == "discover-caller-ip" {
		c.IP = getip2()
	}
	err = CreateSecurityGroups(system, c.NamePrefix, c.IP, c.Ports, c.VPC, "gcp")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *DestroyFirewallCmd) Execute(args []string) error {
	cmd := []string{"config", "gcp", "delete-firewall-rules"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = DeleteSecurityGroups(system, c.NamePrefix, c.All, "gcp")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *LockFirewallCmd) Execute(args []string) error {
	cmd := []string{"config", "gcp", "lock-firewall-rules"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = LockSecurityGroups(system, c.NamePrefix, c.IP, c.Ports, "gcp")
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}
