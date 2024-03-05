package main

import (
	"log"
	"os"
)

type configGcpCmd struct {
	EnableServices   enableServicesCmd  `command:"enable-services" subcommands-optional:"true" description:"enable GCP cloud APIs and services required for AeroLab" webicon:"fas fa-check"`
	DestroySecGroups destroyFirewallCmd `command:"delete-firewall-rules" subcommands-optional:"true" description:"delete aerolab-managed firewall rules" webicon:"fas fa-trash" invwebforce:"true"`
	LockSecGroups    lockFirewallCmd    `command:"lock-firewall-rules" subcommands-optional:"true" description:"lock the client firewall rules so that AMS/vscode are only accessible from a set IP" webicon:"fas fa-lock" invwebforce:"true"`
	CreateSecGroups  createFirewallCmd  `command:"create-firewall-rules" subcommands-optional:"true" description:"create AeroLab-managed firewall rules" webicon:"fas fa-circle-plus" invwebforce:"true"`
	ListSecGroups    listFirewallCmd    `command:"list-firewall-rules" subcommands-optional:"true" description:"list current aerolab-managed firewall rules" webicon:"fas fa-list"`
	ExpiryInstall    expiryInstallCmd   `command:"expiry-install" subcommands-optional:"true" description:"install the expiry system scheduler and lambda with the required IAM roles" webicon:"fas fa-plus" invwebforce:"true"`
	ExpiryRemove     expiryRemoveCmd    `command:"expiry-remove" subcommands-optional:"true" description:"remove the expiry system scheduler, lambda and created IAM roles" webicon:"fas fa-minus" invwebforce:"true"`
	ExpiryCheckFreq  expiryCheckFreqCmd `command:"expiry-run-frequency" subcommands-optional:"true" description:"adjust how often the scheduler runs the expiry check lambda" webicon:"fas fa-gauge" invwebforce:"true"`
	Help             helpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *configGcpCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type enableServicesCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *enableServicesCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "gcp" {
		return logFatal("required backend type to be GCP")
	}
	err := b.EnableServices()
	if err != nil {
		return err
	}
	return nil
}

type listFirewallCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type destroyFirewallCmd struct {
	NamePrefix string  `short:"n" long:"name" description:"Name to use for the firewall" default:"aerolab-managed-external"`
	Internal   bool    `short:"i" long:"internal" description:"Also remove the internal firewall rule if it exists"`
	Help       helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type createFirewallCmd struct {
	NamePrefix string   `short:"n" long:"name" description:"Name to use for the firewall" default:"aerolab-managed-external"`
	Ports      []string `short:"p" long:"port" description:"extra port to open, can be specified multiple times; default: 3000, 80, 443, 8080, 8888, 9200, 22"`
	NoDefaults bool     `short:"d" long:"no-defaults" description:"set to prevent default ports from being opened"`
	Help       helpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

type lockFirewallCmd struct {
	NamePrefix string   `short:"n" long:"name" description:"Name to use for the firewall" default:"aerolab-managed-external"`
	IP         string   `short:"i" long:"ip" description:"set the IP mask to allow access, eg 0.0.0.0/0 or 1.2.3.4/32 or 10.11.12.13" default:"discover-caller-ip"`
	Ports      []string `short:"p" long:"port" description:"extra port to open, can be specified multiple times; default: 3000, 80, 443, 8080, 8888, 9200, 22"`
	NoDefaults bool     `short:"d" long:"no-defaults" description:"set to prevent default ports from being opened"`
	Help       helpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *listFirewallCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "gcp" {
		return logFatal("required backend type to be GCP")
	}
	err := b.ListSecurityGroups()
	if err != nil {
		return err
	}
	return nil
}

func (c *createFirewallCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "gcp" {
		return logFatal("required backend type to be GCP")
	}
	log.Print("Creating firewall rules")
	err := b.CreateSecurityGroups("", c.NamePrefix, false, c.Ports, c.NoDefaults)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}

func (c *destroyFirewallCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "gcp" {
		return logFatal("required backend type to be GCP")
	}
	log.Print("Removing firewall rules")
	err := b.DeleteSecurityGroups("", c.NamePrefix, c.Internal)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}

func (c *lockFirewallCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "gcp" {
		return logFatal("required backend type to be GCP")
	}
	log.Print("Locking firewall rules")
	err := b.LockSecurityGroups(c.IP, true, "", c.NamePrefix, false, c.Ports, c.NoDefaults)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}
