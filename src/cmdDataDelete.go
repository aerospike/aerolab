package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"strings"
)

type dataDeleteCmd struct {
	dataInsertNsSetCmd
	dataInsertPkCmd
	dataInsertCommonCmd
	Durable bool `short:"D" long:"durable-delete" description:"if set, will use durable deletes"`
	dataInsertSelectorCmd
	Help helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *dataDeleteCmd) Execute(args []string) error {
	return c.delete(args)
}

func (c *dataDeleteCmd) delete(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
	log.Print("Running data.delete")
	if c.RunJson != "" {
		jf, err := os.ReadFile(c.RunJson)
		if err != nil {
			return err
		}
		err = json.Unmarshal(jf, c)
		if err != nil {
			return err
		}
	}
	if c.RunDirect {
		var err error
		log.Print("Delete start")
		log.Printf("namespace=%s set=%s pk_start_key=%s%d pk_end_key=%s%d", c.Namespace, c.Set, c.PkPrefix, c.PkStartNumber, c.PkPrefix, c.PkEndNumber)
		switch c.Version {
		case "8":
			err = c.delete7(args)
		case "7":
			err = c.delete6(args)
		case "5":
			err = c.delete5(args)
		case "4":
			err = c.delete4(args)
		default:
			err = errors.New("aerospike client version does not exist")
		}
		if err == nil {
			log.Print("Delete done")
		}
		return err
	}
	if b == nil {
		return logFatal("Invalid backend")
	}
	err := b.Init()
	if err != nil {
		return logFatal("Could not init backend: %s", err)
	}
	seedNode, err := c.checkSeedPort()
	if err != nil {
		return err
	}
	if a.opts.Config.Backend.Type == "docker" {
		found := false
		for _, arg := range os.Args[1:] {
			if strings.HasPrefix(arg, "-g") || strings.HasPrefix(arg, "--seed-node") {
				found = true
				break
			}
		}
		if !found {
			c.SeedNode = seedNode
		}
	}
	log.Print("Unpacking start")
	c.RunDirect = true
	data, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := c.unpack("delete", data); err != nil {
		return err
	}
	log.Print("Complete")
	return nil
}

func (c *dataInsertSelectorCmd) checkSeedPort() (string, error) {
	if a.opts.Config.Backend.Type != "docker" {
		return c.SeedNode, nil
	}
	if c.SeedNode != "127.0.0.1:3000" {
		return c.SeedNode, nil
	}
	inv, err := b.Inventory("", []int{InventoryItemClusters})
	if err != nil {
		return c.SeedNode, err
	}
	b.WorkOnServers()
	if c.IsClient {
		b.WorkOnClients()
	}
	for _, item := range inv.Clusters {
		if item.ClusterName == c.ClusterName.String() && item.NodeNo == strconv.Itoa(c.Node.Int()) && item.DockerExposePorts != "" {
			return "127.0.0.1:" + item.DockerExposePorts, nil
		}
	}
	return c.SeedNode, nil
}
