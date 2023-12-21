package main

import (
	"errors"
	"log"
	"os"
	"strings"
)

type configAwsCmd struct {
	DestroySecGroups destroySecGroupsCmd `command:"delete-security-groups" subcommands-optional:"true" description:"delete aerolab-managed security groups"`
	LockSecGroups    lockSecGroupsCmd    `command:"lock-security-groups" subcommands-optional:"true" description:"lock the client security groups so that AMS/vscode are only accessible from a set IP"`
	CreateSecGroups  createSecGroupsCmd  `command:"create-security-groups" subcommands-optional:"true" description:"create AeroLab-managed security groups in a given VPC"`
	ListSecGroups    listSecGroupsCmd    `command:"list-security-groups" subcommands-optional:"true" description:"list current aerolab-managed security groups"`
	ListSubnets      listSubnetsCmd      `command:"list-subnets" subcommands-optional:"true" description:"list VPCs and subnets in the current region"`
	ExpiryInstall    expiryInstallCmd    `command:"expiry-install" subcommands-optional:"true" description:"install the expiry system scheduler and lambda with the required IAM roles"`
	ExpiryRemove     expiryRemoveCmd     `command:"expiry-remove" subcommands-optional:"true" description:"remove the expiry system scheduler, lambda and created IAM roles"`
	ExpiryCheckFreq  expiryCheckFreqCmd  `command:"expiry-run-frequency" subcommands-optional:"true" description:"adjust how often the scheduler runs the expiry check lambda"`
	Help             helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *configAwsCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type expiryInstallCmd struct {
	Frequency int                 `short:"f" long:"frequency" description:"Scheduler frequency in minutes" default:"10"`
	Gcp       expiryInstallCmdGcp `no-flag:"true"`
	Help      helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

type expiryInstallCmdGcp struct {
	Region string `long:"region" description:"region to deploy the function to"`
}

func init() {
	addBackendSwitch("config.gcp.expiry-install", "gcp", &a.opts.Config.Gcp.ExpiryInstall.Gcp)
	addBackendSwitch("config.gcp.expiry-remove", "gcp", &a.opts.Config.Gcp.ExpiryRemove.Gcp)
}

func (c *expiryInstallCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running config." + a.opts.Config.Backend.Type + ".expiry-install")
	if a.opts.Config.Backend.Type == "docker" {
		return logFatal("required backend type to be AWS|GCP")
	}
	deployRegion := strings.Split(c.Gcp.Region, "-")
	if len(deployRegion) > 2 {
		deployRegion = deployRegion[:len(deployRegion)-1]
	}
	err := b.ExpiriesSystemInstall(c.Frequency, strings.Join(deployRegion, "-"))
	if err != nil {
		return errors.New(err.Error())
	}
	log.Println("Done")
	return nil
}

type expiryRemoveCmd struct {
	Gcp  expiryInstallCmdGcp `no-flag:"true"`
	Help helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *expiryRemoveCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running config." + a.opts.Config.Backend.Type + ".expiry-remove")
	if a.opts.Config.Backend.Type == "docker" {
		return logFatal("required backend type to be AWS|GCP")
	}
	err := b.ExpiriesSystemRemove(c.Gcp.Region)
	if err != nil {
		return errors.New(err.Error())
	}
	log.Println("Done")
	return nil
}

type expiryCheckFreqCmd struct {
	Frequency int     `short:"f" long:"frequency" description:"Scheduler frequency in minutes" default:"10"`
	Help      helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *expiryCheckFreqCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Running config." + a.opts.Config.Backend.Type + ".expiry-run-frequency")
	if a.opts.Config.Backend.Type == "docker" {
		return logFatal("required backend type to be AWS|GCP")
	}
	err := b.ExpiriesSystemFrequency(c.Frequency)
	if err != nil {
		return errors.New(err.Error())
	}
	log.Println("Done")
	return nil
}

type listSecGroupsCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type listSubnetsCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type destroySecGroupsCmd struct {
	NamePrefix string  `short:"n" long:"name" description:"Name prefix to use for the firewall" default:"AeroLab"`
	Internal   bool    `short:"i" long:"internal" description:"Also remove the internal firewall rule if it exists"`
	VPC        string  `short:"v" long:"vpc" description:"vpc ID; default: use default VPC" default:""`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type createSecGroupsCmd struct {
	NamePrefix string  `short:"n" long:"name" description:"Name prefix to use for the firewall" default:"AeroLab"`
	VPC        string  `short:"v" long:"vpc" description:"vpc ID; default: use default VPC" default:""`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type lockSecGroupsCmd struct {
	NamePrefix string  `short:"n" long:"name" description:"Name prefix to use for the firewall" default:"AeroLab"`
	IP         string  `short:"i" long:"ip" description:"set the IP mask to allow access, eg 0.0.0.0/0 or 1.2.3.4/32 or 10.11.12.13" default:"discover-caller-ip"`
	VPC        string  `short:"v" long:"vpc" description:"VPC to handle sec groups for; default: default-VPC" default:""`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *listSecGroupsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "aws" {
		return logFatal("required backend type to be AWS")
	}
	err := b.ListSecurityGroups()
	if err != nil {
		return err
	}
	return nil
}

func (c *listSubnetsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "aws" {
		return logFatal("required backend type to be AWS")
	}
	err := b.ListSubnets()
	if err != nil {
		return err
	}
	return nil
}

func (c *createSecGroupsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "aws" {
		return logFatal("required backend type to be AWS")
	}
	log.Print("Creating security groups")
	err := b.CreateSecurityGroups(c.VPC, c.NamePrefix, false)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}

func (c *destroySecGroupsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "aws" {
		return logFatal("required backend type to be AWS")
	}
	log.Print("Removing security groups")
	err := b.DeleteSecurityGroups(c.VPC, c.NamePrefix, c.Internal)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}

func (c *lockSecGroupsCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "aws" {
		return logFatal("required backend type to be AWS")
	}
	log.Print("Locking security groups")
	err := b.LockSecurityGroups(c.IP, true, c.VPC, c.NamePrefix, false)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}
