package main

import (
	"log"
	"os"
)

type configAwsCmd struct {
	DestroySecGroups destroySecGroupsCmd `command:"delete-security-groups" subcommands-optional:"true" description:"delete aerolab-managed security groups"`
	LockSecGroups    lockSecGroupsCmd    `command:"lock-security-groups" subcommands-optional:"true" description:"lock the client security groups so that AMS/vscode are only accessible from a set IP"`
	CreateSecGroups  createSecGroupsCmd  `command:"create-security-groups" subcommands-optional:"true" description:"create AeroLab-managed security groups in a given VPC"`
	ListSecGroups    listSecGroupsCmd    `command:"list-security-groups" subcommands-optional:"true" description:"list current aerolab-managed security groups"`
	ListSubnets      listSubnetsCmd      `command:"list-subnets" subcommands-optional:"true" description:"list VPCs and subnets in the current region"`
	Help             helpCmd             `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *configAwsCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type listSecGroupsCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type listSubnetsCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type destroySecGroupsCmd struct {
	VPC  string  `short:"v" long:"vpc" description:"vpc ID; default: use default VPC" default:""`
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type createSecGroupsCmd struct {
	VPC  string  `short:"v" long:"vpc" description:"vpc ID; default: use default VPC" default:""`
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type lockSecGroupsCmd struct {
	IP   string  `short:"i" long:"ip" description:"set the IP mask to allow access, eg 0.0.0.0/0 or 1.2.3.4/32 or 10.11.12.13" default:"discover-caller-ip"`
	Ssh  bool    `short:"s" long:"ssh" description:"set to also lock port 22 SSH to the given IP/mask for server and client groups"`
	VPC  string  `short:"v" long:"vpc" description:"VPC to handle sec groups for; default: default-VPC" default:""`
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
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
	err := b.CreateSecurityGroups(c.VPC)
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
	err := b.DeleteSecurityGroups(c.VPC)
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
	err := b.LockSecurityGroups(c.IP, c.Ssh, c.VPC)
	if err != nil {
		return err
	}
	log.Print("Done")
	return nil
}