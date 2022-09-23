# Getting started

## One-time setup

### Docker

Follow the below if using the docker backend.

#### Install docker

Use one of the below methods to install docker:

* install `docker desktop` on your machine
* install docker using `minikube` on your machine
* use the [docker-amd64-mac-m1](https://github.com/aerospike-community/docker-amd64-mac-m1) (works on Intel Mac as well)

#### Start docker

Start docker and ensure it's running by executing `docker version`

#### Configure disk, RAM and CPU resources

If using docker-desktop, in the docker tray-icon, go to "Preferences". Configure the required disk, RAM and CPU resources. At least 2 cores and 2 GB of RAM is recommended for a single-node cluster.

### AWS

Follow the below if using the AWS backend.

#### Configure aws-cli

Follow [this manual](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) to install the AWS CLI.

Run `aws configure` to configure basic access to AWS.

### Download aerolab binary from the releases page

Head to the releases page and download either `aerolab-macos` or `aerolab-linux`.

#### MacOS

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

### First run

#### First run will print help page asking for backend configuration. Do so by following thr printed help page:

```bash
% aerolab

Create a config file and select a backend first using one of:

$ aerolab config backend -t docker
$ aerolab config backend -t aws [-r region] [-p /custom/path/to/store/ssh/keys/in/]

Default file path is ${HOME}/.aerolab.conf

To specify a custom configuration file, set the environment variable:
   $ export AEROLAB_CONFIG_FILE=/path/to/file.conf
```

### Configuring defaults

Default configuration can be changed. If the defaults are adjusted, the new values will be used as defaults. These can still be changed at runtime by specifying command-line switches.

Command | Description
--- | ---
`aerolab config defaults help` | print help for using the defaults command
`aerolab config defaults` | print all defaults
`aerolab config defaults -o` | print only the defaults that have been adjusted in the config file
`aerolab config defaults -k '*Features*'` | print all defaults containing the word `Features`
`aerolab config defaults -k '*.HeartbeatMode' -v mesh` | adjust `HeartbeatMode` default to `mesh` for all available commands

#### Getting started - configuration basics

It's a good idea to configure the basics so as not to have to use the command line switches each time.

If using a custom features file: `aerolab config defaults -k '*FeaturesFilePath' -v /path/to/features.conf`

Make aerolab adjust `aerospike.conf` to always use `mesh` heartbeat modes, unless specifically overwritten in the command line: `aerolab config defaults -k '*.HeartbeatMode' -v mesh`

## Basic usage

### Deploy a cluster called 'testme' with 5 nodes
```
aerolab cluster create --name=testme --count=5
```

### Attach to node 2 in that cluster
```
aerolab attach shell --name=testme --node=2
root@node:/ $ service aerospike status
Aerospike running
root@node:/ $ service aerospike stop
Stopping Aerospike ... OK
root@node:/ $ service aerospike start
Starting Aerospike ... OK
root@node:/ $ exit
```

### Run asadm info command on node 2
```
$ aerolab attach shell --name=testme --node=2 -- asadm -e info
$ aerolab attach asadm --name=testme --node=2 -- -e info
```

### Run asinfo on all nodes
```
$ aerolab attach asinfo --name=testme --node=all -- -v service
$ aerolab attach shell --name=testme --node=all -- asinfo -v service
```

### Destroy the cluster, force stopping
```
$ aerolab cluster destroy --name=testme -f
```

### Get help on commands list

```
aerolab help
aerolab {command} help
aerolab {command} {subcommand} help
```
