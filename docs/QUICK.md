### Quick Start

#### One-time setup

##### 1. Install docker on mac from: https://store.docker.com/editions/community/docker-ce-desktop-mac

##### 2. Run docker (start Docker application)

##### 3. In the docker tray-icon, go to "Preferences", give the Disk 64GB and lots of RAM+CPU cores

##### 4. Download aerolab binary on mac from: https://github.com/citrusleaf/opstools/blob/master/Aero-Lab_Quick_Cluster_Spinup/v2/bin/osx-aio/aerolab

##### 5. Make it executable
```
$ chmod 755 aerolab
```

##### 6. Create a 'common' config file so we don't have to type in the username and password for downloads all the time when making clusters (aerospike enterprise download password)
```bash
$ cat ~/aero-lab-common.conf 
[Common]
Username="USER"
Password="PASS"
```
Also, need a features.conf file, which internally you can get from https://newkey.aerospike.com/gen/ Otherwise, you get an error:

```bash
      Jan 29 19:14:55+0000 AERO-LAB[3224]: WARN     WARNING: you are attempting to install version 4.6+ and did not provide feature.conf file. This will not work. You can either provide a feature file by using the '-f' switch, or inside your ~/aero-lab-common.conf
```

#### Use it forever

##### 1. Start docker if not running on your mac

##### 2. Run aerolab and be happy

Give me a hint how to run aerolab here.
Simply typing ./aerolab fails because there is no command:
```bash
aerolab % /Users/alexlange/Desktop/aerolab/aerolab-osx-aio            
Jan 29 19:12:39+0000 AERO-LAB[3167]: INFO     No command specified. Try running: /Users/alexlange/Desktop/aerolab/aerolab-osx-aio help
```

###### Deploy a cluster called 'testme' with 5 nodes and using mesh
```
$ ./aerolab make-cluster --name=testme --count=5 --mode=mesh
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Performing sanity checks
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Checking if version template already exists
Nov 04 12:45:08+0000 AERO-LAB[97520]: INFO     Checking aerospike version
Nov 04 12:45:15+0000 AERO-LAB[97520]: INFO     Starting deployment
Nov 04 12:45:18+0000 AERO-LAB[97520]: INFO     Done
```

###### Attach to node 2 in that cluster
```
$ ./aerolab node-attach --name=testme --node=2
root@node:/ $ service aerospike status
Aerospike running
root@node:/ $ service aerospike restart
Restarting Aerospike ... OK
root@node:/ $ exit
```

###### Destroy the cluster, force stop too!
```
$ ./aerolab cluster-destroy --name=testme -f 1
```

###### Get help on commands list
```
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
		Destroy cluster machines
	cluster-list
		List currently existing clusters and templates
	cluster-grow
		Deploy more nodes in a specific cluster
...
```

###### Get command help
```
$ ./aerolab make-cluster help
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
