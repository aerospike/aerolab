package main

import (
	"github.com/aerospike/aerolab/confeditor"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type confGeneratorCmd struct {
	Path          flags.Filename `short:"f" long:"filename" description:"file name to read/write/generate" default:"aerospike.conf"`
	DisableColors bool           `short:"c" long:"--no-colors" description:"set to operate in no-color mode"`
	Help          helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *confGeneratorCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
	e := &confeditor.Editor{
		Path:   string(c.Path),
		Colors: !c.DisableColors,
	}
	return e.Run()
}
