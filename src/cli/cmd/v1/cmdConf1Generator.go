package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/aerospike/aerolab/pkg/backend/backends"
	"github.com/aerospike/aerolab/pkg/conf/aerospike/confeditor"
	"github.com/aerospike/aerolab/pkg/conf/aerospike/confeditor7"
	flags "github.com/rglonek/go-flags"
	"github.com/rglonek/logger"
)

type ConfGeneratorCmd struct {
	Path         flags.Filename `short:"f" long:"filename" description:"file name to read/write/generate" default:"aerospike.conf"`
	Pre7         bool           `short:"6" long:"pre-7" description:"set to to generator for pre-version-7 aerospike"`
	EnableColors bool           `short:"c" long:"colors" description:"set to operate in color mode"`
	Help         HelpCmd        `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *ConfGeneratorCmd) Execute(args []string) error {
	cmd := []string{"conf", "generate"}
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: false}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	defer UpdateDiskCache(system)
	err = c.CreateConf(system, nil, system.Logger, args)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}

	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

func (c *ConfGeneratorCmd) CreateConf(system *System, inventory *backends.Inventory, logger *logger.Logger, args []string) error {
	/* we do not use system in this
	if system == nil {
		var err error
		system, err = Initialize(&Init{InitBackend: false, ExistingInventory: inventory}, []string{"conf", "generate"}, c, args...)
		if err != nil {
			return err
		}
	}
	*/
	if _, err := os.Stat(string(c.Path)); err == nil {
		fmt.Printf("WARNING!\nFile '%s' already exists. If you continue, aerolab will attempt to read and parse the file.\nNote that aerolab will not automatically recognise whether the config is for aerospike 7+ or older. The `--pre-7` flag must be provided for older versions.\n\nPress ENTER to continue or CTRL+C to abort.\n", c.Path)
		var ignoreMe string
		fmt.Scanln(&ignoreMe)
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
