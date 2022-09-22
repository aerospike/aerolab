package main

import "fmt"

type rosterCheatCmd struct {
	Namespace string  `short:"m" long:"namespace" description:"Namespace name" default:"test"`
	Help      helpCmd `command:"help" subcommands-optional:"true" description:"Print help"`
}

func (c *rosterCheatCmd) Execute(args []string) error {
	if earlyProcessV2(args, false) {
		return nil
	}
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
	return nil
}
