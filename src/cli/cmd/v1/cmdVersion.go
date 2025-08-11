package cmd

import (
	"fmt"
)

type VersionCmd struct {
	Help HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *VersionCmd) Execute(args []string) error {
	system, err := Initialize(&Init{InitBackend: false, UpgradeCheck: true}, []string{"version"}, c, args...)
	if err != nil {
		return Error(err, system, []string{"version"}, c, args)
	}
	return Error(c.GetVersion(), system, []string{"version"}, c, args)
}

func (c *VersionCmd) GetVersion() error {
	_, _, _, versionString := GetAerolabVersion()
	fmt.Println(versionString)
	return nil
}
