package main

import (
	"log"
	"os"
)

type volumeCmd struct {
	Create volumeCreateCmd `command:"create" subcommands-optional:"true" description:"Create a volume"`
	List   volumeListCmd   `command:"list" subcommands-optional:"true" description:"List volumes"`
	Mount  volumeMountCmd  `command:"mount" subcommands-optional:"true" description:"Mount a volume on a node"`
	Delete volumeDeleteCmd `command:"delete" subcommands-optional:"true" description:"Delete a volume"`
	Help   helpCmd         `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeCmd) Execute(args []string) error {
	a.parser.WriteHelp(os.Stderr)
	os.Exit(1)
	return nil
}

type volumeListCmd struct {
	Json    bool    `short:"j" long:"json" description:"Provide output in json format"`
	Owner   string  `long:"owner" description:"filter by owner tag/label"`
	NoPager bool    `long:"no-pager" description:"set to disable vertical and horizontal pager"`
	Help    helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeListCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	a.opts.Inventory.List.Json = c.Json
	a.opts.Inventory.List.Owner = c.Owner
	a.opts.Inventory.List.NoPager = c.NoPager
	return a.opts.Inventory.List.run(false, false, false, false, false, inventoryShowVolumes)
}

type volumeCreateCmd struct {
	Name string   `short:"n" long:"name" description:"EFS Name" default:"agi"`
	Zone string   `short:"z" long:"zone" description:"Full Availability Zone name; if provided, will define a one-zone volume; default {REGION}a"`
	Tags []string `short:"t" long:"tag" description:"tag as key=value; can be specified multiple times"`
	Help helpCmd  `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeCreateCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Creating volume")
	err := b.CreateVolume(c.Name, c.Zone, c.Tags)
	if err != nil {
		return err
	}
	log.Println("Done")
	return nil
}

type volumeMountCmd struct {
	Name        string  `short:"n" long:"name" description:"EFS Name" default:"agi"`
	ClusterName string  `short:"N" long:"cluster-name" description:"Cluster/Client Name on which to mount" default:"agi"`
	IsClient    bool    `short:"c" long:"is-client" description:"Specify mounting on client instead of cluster"`
	Nodes       string  `short:"l" long:"nodes" description:"Nodes to mount on; default:all"`
	LocalPath   string  `short:"p" long:"mount-path" description:"Path on the node to mount to" default:"/mnt/{EFS_NAME}"`
	EfsPath     string  `short:"P" long:"volume-path" description:"Volume path to mount" default:"/"`
	Help        helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeMountCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	// TODO: be smart, check if mount target exists for a given instance AZ, if not, create it
	// TODO: if a mount target exists, check if it belongs to the correct security groups and subnet
	// TODO: if wrong security groups, fix; if wrong subnet, create new mount target
	// TODO: install EFS UTILS, add target to /etc/fstab, mount -a
	return nil
}

type volumeDeleteCmd struct {
	Name string  `short:"n" long:"name" description:"EFS Name" default:"agi"`
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *volumeDeleteCmd) Execute(args []string) error {
	if earlyProcess(args) {
		return nil
	}
	log.Println("Deleting volume")
	err := b.DeleteVolume(c.Name)
	if err != nil {
		return err
	}
	log.Println("Done")
	return nil
}
