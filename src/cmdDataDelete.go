package main

import (
	"errors"
	"log"
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
	if c.RunDirect {
		log.Print("Delete start")
		defer log.Print("Delete done")
		switch c.Version {
		case "6":
			return c.delete6(args)
		case "5":
			return c.delete5(args)
		case "4":
			return c.delete4(args)
		default:
			return errors.New("aerospike client version does not exist")
		}
	}
	if b == nil {
		return logFatal("Invalid backend")
	}
	err := b.Init()
	if err != nil {
		return logFatal("Could not init backend: %s", err)
	}
	log.Print("Unpacking start")
	if err := c.unpack(args); err != nil {
		return err
	}
	log.Print("Unpacking done")
	return nil
}
