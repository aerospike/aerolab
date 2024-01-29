package main

import (
	"github.com/aerospike/aerolab/confeditor"
	"github.com/aerospike/aerolab/confeditor7"
	flags "github.com/rglonek/jeddevdk-goflags"
)

type confGeneratorCmd struct {
	Path         flags.Filename `short:"f" long:"filename" description:"file name to read/write/generate" default:"aerospike.conf"`
	Pre7         bool           `short:"6" long:"pre-7" description:"set to to generator for pre-version-7 aerospike"`
	EnableColors bool           `short:"c" long:"colors" description:"set to operate in color mode"`
	Help         helpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *confGeneratorCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
	if c.Pre7 {
		e := &confeditor.Editor{
			Path:   string(c.Path),
			Colors: c.EnableColors,
		}
		return e.Run()
	}
	e := &confeditor7.Editor{
		Path:   string(c.Path),
		Colors: c.EnableColors,
	}
	return e.Run()
}
