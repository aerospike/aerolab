# Make cluster with strong consistency

### Create cluster, shipping custom config and features file

```bash
$ ./aerolab make-cluster -c 3 -o templates/strong-consistency.conf -f features.conf
Nov 06 09:25:12+0000 AERO-LAB[13734]: INFO     Performing sanity checks
Nov 06 09:25:13+0000 AERO-LAB[13734]: INFO     Checking if version template already exists
Nov 06 09:25:13+0000 AERO-LAB[13734]: INFO     Checking aerospike version
Nov 06 09:25:25+0000 AERO-LAB[13734]: INFO     Starting deployment
Nov 06 09:25:30+0000 AERO-LAB[13734]: INFO     Done
```

### Get quick help on SC commands to execute for roster creation
```bash
$ ./aerolab sc-help
To enable Strong Consistency or add nodes to an SC cluster:
$ ./node-attach
$ asadm -e "asinfo -v 'roster:namespace=bar'"
$ asadm -e "asinfo -v 'roster-set:namespace=bar;nodes=[observed nodes list]'"
$ asadm -e "asinfo -v 'roster:namespace=bar'"
$ asadm -e "asinfo -v 'recluster:namespace=bar'"
$ asadm -e "asinfo -v 'roster:namespace=bar'"

To remove a node:
$ ./stop-aerospike
Stop the node to be removed
$ ./node-attach
Attach to a running node
$ asadm -e "show stat like unavailable for bar -flip"
Make sure there are no unavailable partitions
$ asadm -e "show stat service like partitions_remain -flip"
Wait for migrations to finish
$ asadm -e "asinfo -v 'roster:namespace=bar'"
$ asadm -e "asinfo -v 'roster-set:namespace=bar;nodes=[observed nodes list]'"
$ asadm -e "asinfo -v 'recluster:namespace=bar'"
$ asadm -e "asinfo -v 'roster:namespace=bar'"

To recover from dead partitions:
$ ./node-attach
$ asadm -e "show stat namespace for bar like dead -flip"
$ asadm -e "asinfo -v 'revive:namespace=bar'"
$ asadm -e "asinfo -v 'recluster:namespace=bar'"
$ asadm -e "show stat namespace for bar like dead -flip"
```

### Connect to a node and run roster commands
```bash
$ ./aerolab node-attach -n myjava
root@4b47d3ff291c:/# asadm -e "asinfo -v 'roster:namespace=bar'"
root@4b47d3ff291c:/# asadm -e "asinfo -v 'roster-set:namespace=bar;nodes=...'"
root@4b47d3ff291c:/# $ asadm -e "asinfo -v 'recluster:namespace=bar'"
```
