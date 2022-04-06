# Make cluster with strong consistency

## Create cluster, shipping custom config and features file

```bash
$ ./aerolab make-cluster -c 3 -o templates/strong-consistency.conf -f features.conf
```

## Get quick help on SC commands to execute for roster creation

```bash
$ ./aerolab sc-help
```

## Connect to a node and run roster commands

```bash
$ ./aerolab node-attach
asadm -e "asinfo -v 'roster:namespace=bar'"
asadm -e "asinfo -v 'roster-set:namespace=bar;nodes=...'"
asadm -e "asinfo -v 'recluster:namespace=bar'"
```
