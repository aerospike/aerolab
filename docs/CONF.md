## Aero-Lab configuration files

Aero-Lab reads configuration files, if they exist from:
* /etc/aero-lab-common.conf
* ~/aero-lab-common.conf

It also reads any config files as specified in --config={filename}

**Command line switches override what's in the config files. You could call config files the "user defined defaults"**

### Format is as follows:

* First optional line specifies command to execute, e.g.
```
command="make-cluster"
```
* The next lines specify parameters for specific command(s). The following example specifies that if make-cluster command runs, cluster name default should be 'me', node count should be 7 and the USERNAME and PASSWORD for downloads are preset.
```
[MakeCluster]
ClusterName="me"
NodeCount=7
Username="USER"
Password="PASS"
```
* Joining the 2 above, we could have this config file, which, when run, will run make-cluster with the preset parameters:
```
command="make-cluster"

[MakeCluster]
ClusterName="me"
NodeCount=7
Username="USER"
Password="PASS"
```
* [Common] is a special type. The common part allows to specify parameters which should always be preset to a specific value. For example, putting the below in ~/aero-lab-common.conf will allow us to always omit the username and password (we can use it in other config files or switches in command line to override if we want to though)
```
[Common]
Username="USER"
Password="PASS"
```

* the following example shows we can also force use of docker instead of LXC on all commands
```
[Common]
Username="USER"
Password="PASS"
DeployOn="docker"
```

### Please note that all values of each command's configuration file format are nicely displayed in aerolab help pages. See the 'Using Help' part for more information