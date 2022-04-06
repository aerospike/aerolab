package main

import "fmt"

func (c *config) F_scHelp() (ret int64, err error) {
	nsName := c.ScHelp.Namespace
	fmt.Printf(`To enable Strong Consistency or add nodes to an SC cluster:
$ ./node-attach
$ asadm -e "asinfo -v 'roster:namespace=%s'"
$ asadm -e "asinfo -v 'roster-set:namespace=%s;nodes=[observed nodes list]'"
$ asadm -e "asinfo -v 'roster:namespace=%s'"
$ asadm -e "asinfo -v 'recluster:namespace=%s'"
$ asadm -e "asinfo -v 'roster:namespace=%s'"

To remove a node:
$ ./stop-aerospike
Stop the node to be removed
$ ./node-attach
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
$ ./node-attach
$ asadm -e "show stat namespace for %s like dead -flip"
$ asadm -e "asinfo -v 'revive:namespace=%s'"
$ asadm -e "asinfo -v 'recluster:namespace=%s'"
$ asadm -e "show stat namespace for %s like dead -flip"
`, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName, nsName)
	return int64(0), nil
}
