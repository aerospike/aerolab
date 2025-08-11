package cmd

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike"
	"github.com/rglonek/logger"
)

type InstallerListVersionsCmd struct {
	Prefix    string  `short:"v" long:"version" description:"Version Prefix to search for" default:""`
	Community bool    `short:"c" long:"community" description:"Set this switch to list community editions"`
	Federal   bool    `short:"f" long:"federal" description:"Set this switch to list federal editions; cancels --community"`
	Reverse   bool    `short:"r" long:"reverse" description:"Reverse-sort the results"`
	Url       bool    `short:"l" long:"show-url" description:"Show direct access url instead of version number"`
	Help      HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *InstallerListVersionsCmd) Execute(args []string) error {
	system, err := Initialize(&Init{InitBackend: false}, []string{"installer", "list-versions"}, c, args...)
	if err != nil {
		return Error(err, system, []string{"installer", "list-versions"}, c, args)
	}
	system.Logger.Info("Listing Aerospike versions")
	err = c.ListVersions(system.Logger, os.Stdout)
	if err != nil {
		return Error(err, system, []string{"installer", "list-versions"}, c, args)
	}
	system.Logger.Info("Done")
	return Error(nil, system, []string{"installer", "list-versions"}, c, args)
}

func (c *InstallerListVersionsCmd) ListVersions(log *logger.Logger, out io.Writer) error {
	edition := "enterprise"
	if c.Community {
		edition = "community"
	} else if c.Federal {
		edition = "federal"
	}
	version := strings.TrimSuffix(c.Prefix, "*")
	products, err := aerospike.GetProducts(time.Second * 10)
	if err != nil {
		return fmt.Errorf("could not get products: %s", err)
	}
	product := products.WithName("aerospike-server-" + edition)
	if product == nil {
		return fmt.Errorf("product not found")
	}
	versions, err := aerospike.GetVersions(time.Second*10, product[0])
	if err != nil {
		return fmt.Errorf("could not get versions: %s", err)
	}
	if version != "" {
		versions = versions.WithNamePrefix(version)
	}
	if len(versions) == 0 {
		return fmt.Errorf("no versions found")
	}

	// reverse order of the versions list
	if !c.Reverse {
		versions = versions.SortOldestFirst()
	}

	// print the versions
	for _, ver := range versions {
		if c.Url {
			fmt.Fprintln(out, ver.Link)
		} else {
			fmt.Fprintln(out, ver.Name)
		}
	}
	return nil
}
