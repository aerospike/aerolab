package main

import (
	"errors"
	"os"
)

type configDockerCmd struct {
	CreateNetwork createNetworkCmd `command:"create-network" subcommands-optional:"true" description:"create a new docker network"`
	DeleteNetwork deleteNetworkCmd `command:"delete-network" subcommands-optional:"true" description:"delete a docker network"`
	ListNetworks  listNetworksCmd  `command:"list-networks" subcommands-optional:"true" description:"list docker networks"`
	PruneNetworks pruneNetworksCmd `command:"prune-networks" subcommands-optional:"true" description:"remove unused docker networks"`
	Help          helpCmd          `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *configDockerCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type pruneNetworksCmd struct {
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type listNetworksCmd struct {
	CSV  bool    `short:"c" long:"csv" description:"csv output"`
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type deleteNetworkCmd struct {
	Name string  `short:"n" long:"name" description:"network name to delete" default:""`
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

type createNetworkCmd struct {
	Name   string  `short:"n" long:"name" description:"network name to create" default:""`
	Driver string  `short:"d" long:"driver" description:"network driver" default:"bridge"`
	Subnet string  `short:"s" long:"subnet" description:"network subnet to create, ex. 172.18.0.0/24 or 172.18.1.0/24" default:"default"`
	MTU    string  `short:"m" long:"mtu" description:"MTU in the network" default:"default"`
	Help   helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *pruneNetworksCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "docker" {
		return logFatal("required backend type to be DOCKER")
	}
	err := b.PruneNetworks()
	if err != nil {
		return err
	}
	return nil
}

func (c *listNetworksCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "docker" {
		return logFatal("required backend type to be DOCKER")
	}
	err := b.ListNetworks(c.CSV, nil)
	if err != nil {
		return err
	}
	return nil
}

func (c *deleteNetworkCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "docker" {
		return logFatal("required backend type to be DOCKER")
	}
	if c.Name == "" {
		return errors.New("name must be specified")
	}
	err := b.DeleteNetwork(c.Name)
	if err != nil {
		return err
	}
	return nil
}

func (c *createNetworkCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	if a.opts.Config.Backend.Type != "docker" {
		return logFatal("required backend type to be DOCKER")
	}
	if c.Name == "" {
		return errors.New("name must be specified")
	}
	if c.Subnet == "default" {
		c.Subnet = ""
	}
	if c.MTU == "default" {
		c.MTU = ""
	}
	err := b.CreateNetwork(c.Name, c.Driver, c.Subnet, c.MTU)
	if err != nil {
		return err
	}
	return nil
}
