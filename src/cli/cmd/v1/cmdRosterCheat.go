package cmd

import (
	"fmt"
	"strings"
)

type RosterCheatCmd struct {
	Namespace string  `short:"m" long:"namespace" description:"Namespace name" default:"test"`
	Help      HelpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *RosterCheatCmd) Execute(args []string) error {
	cmd := []string{"roster", "cheat"}
	system, err := Initialize(&Init{InitBackend: true, UpgradeCheck: true}, cmd, c, args...)
	if err != nil {
		return Error(err, system, cmd, c, args)
	}
	system.Logger.Info("Running %s", strings.Join(cmd, "."))

	c.ShowCheatSheet()
	system.Logger.Info("Done")
	return Error(nil, system, cmd, c, args)
}

// ShowCheatSheet displays a quick strong consistency cheat-sheet.
// This function provides helpful commands for managing strong consistency
// in Aerospike clusters, including enabling SC, removing nodes, and
// recovering from dead partitions.
func (c *RosterCheatCmd) ShowCheatSheet() {
	nsName := c.Namespace
	fmt.Printf(`To enable Strong Consistency or add nodes to an SC cluster:
$ aerolab attach shell
$ asadm -e "asinfo -v 'roster:namespace=%s'"
$ asadm -e "asinfo -v 'roster-set:namespace=%s;nodes=[observed nodes list]'"
$ asadm -e "asinfo -v 'roster:namespace=%s'"
$ asadm -e "asinfo -v 'recluster:namespace=%s'"
$ asadm -e "asinfo -v 'roster:namespace=%s'"

To remove a node:
$ aerolab aerospike stop -l 2
Stop the node to be removed
$ aerolab attach shell -l 1
Attach to a running node
$ asadm -e "show stat like unavailable for %s -flip"
Make sure there are no unavailable partitions
$ asadm -e "show stat service like partitions_remain -flip"
Wait for migrations to finish
$ asadm -e "asinfo -v 'roster:namespace=%s'"
$ asadm -e "asinfo -v 'roster-set:namespace=%s;nodes=[observed nodes list]'"
$ asadm -e "asinfo -v 'recluster:namespace=%s'"
$ asadm -e "asinfo -v 'roster:namespace=%s'"

To recover from dead partitions:
$ aerolab attach shell
$ asadm -e "show stat namespace for %s like dead -flip"
$ asadm -e "asinfo -v 'revive:namespace=%s'"
$ asadm -e "asinfo -v 'recluster:namespace=%s'"
$ asadm -e "show stat namespace for %s like dead -flip"
`, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName)
}
