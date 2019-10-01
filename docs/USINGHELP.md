#### Using help in aerolab

The help features of aerolab are written in a way that using and exploring aerolab should be simple and easy (let me know if it isn't!)

##### Get help on all commands

```bash
$ ./aerolab help
Usage: ./aerolab {command} [options] [-- {tail}]

Commands:
	interactive
		Enter interactive mode
	make-cluster
		Create a new cluster
	cluster-start
		Start cluster machines
	cluster-stop
		Stop cluster machines
	cluster-destroy
...
```

##### Get help on a specific command
```bash
$ ./aerolab cluster-destroy help
Command: cluster-destroy

-n | --name                	 : Cluster name (default=mydc)
-l | --nodes               	 : Nodes list, comma separated. Empty=ALL (default=)
-f | --force               	 : set to --force=1 to force stop before destroy (default=0)
-e | --deploy-on           	 : Deploy where (aws|docker|lxc) (default=)
...
```

##### Get help on a specific command half-way through writing it (wops, forgot some switches)
```bash
$ ./aerolab make-cluster -n test -c 4 help
Command: make-cluster

-n | --name                	 : Cluster name (default=mydc)
-c | --count               	 : Number of nodes to create (default=1)
-v | --aerospike-version   	 : Version of aerospike to use (add 'c' to denote community, e.g. 3.13.0.1c) (default=latest)
-d | --distro              	 : OS distro to use. One of: ubuntu, rhel. rhel (default=ubuntu)
...
```

##### This also works
```bash
$ ./aerolab help make-cluster
```