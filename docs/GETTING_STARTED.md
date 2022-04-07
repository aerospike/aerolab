# Getting started

## One-time setup

### Install docker

Use one of the below methods to install docker:

* install `docker desktop` on your machine
* install docker using `minikube` on your machine

### Start docker

Start docker and ensure it's running by executing `docker version`

### Configure disk, RAM and CPU resources

If using docker-desktop, in the docker tray-icon, go to "Preferences". Configure the required disk, RAM and CPU resources. At least 2 cores and 2 GB of RAM is recommended for a single-node cluster.

### Download aerolab binary from the releases page

Head to the releases page and download either `aerolab-macos` or `aerolab-linux`.

#### MacOS

Since aerolab-macos is not macos-signed, the easiest way around it is by performing a copy using the `cat` command, like so:

```
mkdir -p /usr/local/bin/
cat aerolab-macos > /usr/local/bin/aerolab
```

#### Linux / WSL2 on windows

```
mkdir -p /usr/local/bin/
mv aerolab-linux /usr/local/bin/aerolab
```

### Make it executable

```
chmod 755 /usr/local/bin/aerolab
```

### Create a 'common' config file

Create a `~/aero-lab-common.conf` file with the following contents to simplify usage:

```bash
[Common]
FeaturesFilePath="/path/to/your/features.conf"
HeartbeatMode="mesh"
```

The `common` section means that these parameters are applied to all functions, for example `make-cluster` and `cluster-grow`

## Basic usage

### Deploy a cluster called 'testme' with 5 nodes
```
$ aerolab make-cluster --name=testme --count=5
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Performing sanity checks
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Checking if version template already exists
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Checking aerospike version
Nov 04 12:45:15+0000 AERO-LAB[97520]: INFO     Starting deployment
Nov 04 12:45:18+0000 AERO-LAB[97520]: INFO     Done
```

### Attach to node 2 in that cluster
```
$ aerolab node-attach --name=testme --node=2
root@node:/ $ service aerospike status
Aerospike running
root@node:/ $ service aerospike stop
Stopping Aerospike ... OK
root@node:/ $ service aerospike start
Starting Aerospike ... OK
root@node:/ $ exit
```

### Run asadm info command


```
$ aerolab node-attach --name=testme -- asadm -e info
```

### Destroy the cluster, force stop too!
```
$ aerolab cluster-destroy --name=testme -f
```

### Get help on commands list
```
$ aerolab help
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
		Destroy cluster machines
	cluster-list
		List currently existing clusters and templates
	cluster-grow
		Deploy more nodes in a specific cluster
...
```

### Get command help
```
$ aerolab make-cluster help
Command: make-cluster

-n | --name                	 : Cluster name (default=mydc)
-c | --count               	 : Number of nodes to create (default=1)
-v | --aerospike-version   	 : Version of aerospike to use (add 'c' to denote community, e.g. 3.13.0.1c) (default=latest)
-d | --distro              	 : OS distro to use. One of: ubuntu, rhel. rhel (default=ubuntu)
-i | --distro-version      	 : Version of distro. E.g. 7, 6 for RHEL/centos, 18.04, 16.04 for ubuntu (default=18.04)
-o | --customconf          	 : Custom config file path to install (default=)
-f | --featurefile         	 : Features file to install (default=)
-m | --mode                	 : Heartbeat mode, values are: mcast|mesh|default. Default:don't touch (default=default)
-a | --mcast-address       	 : Multicast address to change to in config file (default=)
-p | --mcast-port          	 : Multicast port to change to in config file (default=)
-s | --start               	 : Auto-start aerospike after creation of cluster (y/n) (default=y)
-e | --deploy-on           	 : Deploy where (aws|docker|lxc) (default=)
-r | --remote-host         	 : Remote host to use for deployment, as user@ip:port (empty=locally) (default=)
-k | --pubkey              	 : Public key to use to login to hosts when installing to remote (default=)
-U | --username            	 : Required for downloading enterprise edition (default=)
-P | --password            	 : Required for downloading enterprise edition (default=)
...
```
