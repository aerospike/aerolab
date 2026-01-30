package cmd

import (
	"os"
	"strings"
)

type ConfigAwsCmd struct {
	ListSecGroups    ListSecGroupsCmd    `command:"list-security-groups" subcommands-optional:"true" description:"list current aerolab-managed security groups" webicon:"fas fa-list"`
	CreateSecGroups  CreateSecGroupsCmd  `command:"create-security-groups" subcommands-optional:"true" description:"create AeroLab-managed security groups in a given VPC" webicon:"fas fa-circle-plus" invwebforce:"true"`
	LockSecGroups    LockSecGroupsCmd    `command:"lock-security-groups" subcommands-optional:"true" description:"lock the client security groups so that AMS/vscode are only accessible from a set IP" webicon:"fas fa-lock" invwebforce:"true"`
	DestroySecGroups DestroySecGroupsCmd `command:"delete-security-groups" subcommands-optional:"true" description:"delete aerolab-managed security groups" webicon:"fas fa-trash" invwebforce:"true"`
	ListSubnets      ListSubnetsCmd      `command:"list-subnets" subcommands-optional:"true" description:"list VPCs and subnets in the current region" webicon:"fas fa-list-ol"`
	ExpiryInstall    ExpiryInstallCmd    `command:"expiry-install" subcommands-optional:"true" description:"install the expiry system scheduler and lambda with the required IAM roles" webicon:"fas fa-plus" invwebforce:"true"`
	ExpiryRemove     ExpiryRemoveCmd     `command:"expiry-remove" subcommands-optional:"true" description:"remove the expiry system scheduler, lambda and created IAM roles" webicon:"fas fa-minus" invwebforce:"true"`
	ExpiryCheckFreq  ExpiryCheckFreqCmd  `command:"expiry-run-frequency" subcommands-optional:"true" description:"adjust how often the scheduler runs the expiry check lambda" webicon:"fas fa-gauge" invwebforce:"true"`
	ExpiryList       ExpiryListCmd       `command:"expiry-list" subcommands-optional:"true" description:"list the expiry systems" webicon:"fas fa-list"`
	Help             HelpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfigAwsCmd) Execute(args []string) error {
	c.Help.Execute(args)
	return nil
}

type ListSecGroupsCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Owner      string   `short:"u" long:"owner" description:"Filter by owner"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type ListSubnetsCmd struct {
	Output     string   `short:"o" long:"output" description:"Output format (text, table, json, json-indent, jq, csv, tsv, html, markdown)" default:"table"`
	TableTheme string   `short:"t" long:"table-theme" description:"Table theme (default, frame, box)" default:"default"`
	SortBy     []string `short:"s" long:"sort-by" description:"Can be specified multiple times. Sort by format: FIELDNAME:asc|dsc|ascnum|dscnum"`
	Pager      bool     `short:"p" long:"pager" description:"Use a pager to display the output"`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type DestroySecGroupsCmd struct {
	NamePrefix string  `short:"n" long:"name" description:"Name prefix to use for the firewall" default:"AeroLab"`
	All        bool    `short:"a" long:"all" description:"Remove all firewalls (ignores name parameter)"`
	VPC        string  `short:"v" long:"vpc" hidden:"true" webhidden:"true" description:"this no longer applies"` // NOTE: obsolete, but kept for backwards compatibility
	Help       HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type CreateSecGroupsCmd struct {
	NamePrefix string   `short:"n" long:"name" description:"Name prefix to use for the firewall" default:"AeroLab"`
	IP         string   `short:"i" long:"ip" description:"set the IP mask to allow access, eg 0.0.0.0/0 or 1.2.3.4/32 or 10.11.12.13" default:"discover-caller-ip"`
	Ports      []string `short:"p" long:"port" description:"ports to open, can be specified multiple times, ex: 3000-3005 or tcp:3000-3005 or udp:3000"`
	NoDefaults bool     `short:"d" long:"no-defaults" hidden:"true" webhidden:"true" description:"this no longer applies"` // NOTE: obsolete, but kept for backwards compatibility
	VPC        string   `short:"v" long:"vpc" description:"vpc ID; default: use default VPC" default:""`
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type LockSecGroupsCmd struct {
	NamePrefix string   `short:"n" long:"name" description:"Name prefix to use for the firewall" default:"AeroLab"`
	IP         string   `short:"i" long:"ip" description:"set the IP mask to allow access, eg 0.0.0.0/0 or 1.2.3.4/32 or 10.11.12.13" default:"discover-caller-ip"`
	VPC        string   `short:"v" long:"vpc" description:"VPC to handle sec groups for; default: default-VPC" default:""`
	Ports      []string `short:"p" long:"port" description:"ports to open, can be specified multiple times, ex: 3000-3005 or tcp:3000-3005 or udp:3000"`
	NoDefaults bool     `short:"d" long:"no-defaults" hidden:"true" webhidden:"true" description:"this no longer applies"` // NOTE: obsolete, but kept for backwards compatibility
	Help       HelpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ListSecGroupsCmd) Execute(args []string) error {
	cmd := []string{"config", "aws", "list-security-groups"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = ListSecurityGroups(system, c.Output, c.TableTheme, c.SortBy, "aws", cmd, c, args, system.Backend.GetInventory(), c.Owner, os.Stdout, c.Pager, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ListSubnetsCmd) Execute(args []string) error {
	cmd := []string{"config", "aws", "list-subnets"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	err = ListSubnets(system, c.Output, c.TableTheme, c.SortBy, "aws", cmd, c, args, system.Backend.GetInventory(), os.Stdout, c.Pager, nil)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *CreateSecGroupsCmd) Execute(args []string) error {
	cmd := []string{"config", "aws", "create-security-groups"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	if c.IP == "discover-caller-ip" {
		c.IP = getip2()
	}
	if !strings.Contains(c.IP, "/") {
		c.IP = c.IP + "/32"
	}
	defer UpdateDiskCache(system)()
	err = CreateSecurityGroups(system, c.NamePrefix, c.IP, c.Ports, c.VPC, "aws", cmd, c, args, system.Backend.GetInventory())
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *DestroySecGroupsCmd) Execute(args []string) error {
	cmd := []string{"config", "aws", "delete-security-groups"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = DeleteSecurityGroups(system, c.NamePrefix, c.All, "aws", cmd, c, args, system.Backend.GetInventory())
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *LockSecGroupsCmd) Execute(args []string) error {
	cmd := []string{"config", "aws", "lock-security-groups"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)()
	err = LockSecurityGroups(system, c.NamePrefix, c.IP, c.Ports, "aws", cmd, c, args, system.Backend.GetInventory())
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}
