package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike"
	"github.com/aerospike/aerolab/pkg/utils/installers/aerospike/jfrog"
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

	// JFrog dev-build mode: enumerate the builds registered under the
	// configured build name via the AQL builds domain. Edition/format/arch
	// are not surfaced at this level because every build run publishes all
	// editions and OS variants; the version prefix (if any) is pushed to
	// JFrog as a "$match" glob on the build number.
	if cfg := jfrog.FromEnv(); cfg != nil {
		_ = edition // edition filter has no effect in JFrog mode
		match := strings.TrimSuffix(version, "-artifacts")
		builds, err := cfg.ListBuilds(context.Background(), match)
		if err != nil {
			return fmt.Errorf("could not list JFrog builds: %s", err)
		}
		if len(builds) == 0 {
			return fmt.Errorf("no JFrog builds found for %q", cfg.BuildName)
		}
		// Every build run publishes a canonical "-artifacts" entry plus a
		// non-suffixed alias. Keep only the "-artifacts" entries (so we
		// list builds that actually have artifacts) and strip the suffix
		// for a clean, copy-pasteable version name.
		builds = builds.OnlyArtifacts()
		if len(builds) == 0 {
			return fmt.Errorf("no JFrog builds with artifacts found for %q", cfg.BuildName)
		}
		// JFrog returns descending-by-number; reverse for oldest-first
		// parity with the public flow unless --reverse was requested.
		if !c.Reverse {
			for i, j := 0, len(builds)-1; i < j; i, j = i+1, j-1 {
				builds[i], builds[j] = builds[j], builds[i]
			}
		}
		for _, b := range builds {
			name := strings.TrimSuffix(b.Number, "-artifacts")
			if c.Url {
				fmt.Fprintln(out, cfg.BuildInfoURL(b.Name, b.Number)) //nolint:errcheck
			} else {
				fmt.Fprintln(out, name) //nolint:errcheck
			}
		}
		return nil
	}

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
			fmt.Fprintln(out, ver.Link) //nolint:errcheck
		} else {
			fmt.Fprintln(out, ver.Name) //nolint:errcheck
		}
	}
	return nil
}
