package main

import (
	"log"
	"os"
)

type configGcpCmd struct {
	DestroySecGroups destroyFirewallCmd `command:"delete-firewall-rules" subcommands-optional:"true" description:"delete aerolab-managed firewall rules"`
	LockSecGroups    lockFirewallCmd    `command:"lock-firewall-rules" subcommands-optional:"true" description:"lock the client firewall rules so that AMS/vscode are only accessible from a set IP"`
	CreateSecGroups  createFirewallCmd  `command:"create-firewall-rules" subcommands-optional:"true" description:"create AeroLab-managed firewall rules"`
	ListSecGroups    listFirewallCmd    `command:"list-firewall-rules" subcommands-optional:"true" description:"list current aerolab-managed firewall rules"`
	Help             helpCmd            `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *configGcpCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type listFirewallCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type destroyFirewallCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type createFirewallCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type lockFirewallCmd struct {
	IP   string  `short:"i" long:"ip" description:"set the IP mask to allow access, eg 0.0.0.0/0 or 1.2.3.4/32 or 10.11.12.13" default:"discover-caller-ip"`
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
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
	log.Print("Creating security groups")
	err := b.CreateSecurityGroups("")
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
	log.Print("Removing security groups")
	err := b.DeleteSecurityGroups("")
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
	log.Print("Locking security groups")
	err := b.LockSecurityGroups(c.IP, true, "")
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}
