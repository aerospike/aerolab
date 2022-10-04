# Getting started

## One-time setup

Follow either the `Docker` or `AWS` manual below, depending on the backend you wish to you. Both backends may be used and `aerolab` may be switched between them.

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

Head to the releases page and download one of the binaries, depending on where you are intending to run aerolab.

Note that aerolab will still be able to deploy Aerospike on both arm and x64 architectures, regardless of which aerolab binary you are using.

Operating System | CPU | File
--- | --- | ---
MacOS | M1 or M2 | `aerolab-macos-arm64.zip`
MacOS | Intel CPU | `aerolab-macos-amd64.zip`
Linux | arm | `aerolab-linux-arm64.zip`
Linux | Intel/Amd | `aerolab-linux-amd64.zip`

#### Install

First unzip the zip file. This will produce a single binary called `aerolab`. Follow the below steps to install:

```
sudo mkdir -p /usr/local/bin/
sudo mv /path/to/aerolab /usr/local/bin/
sudo chmod 755 /usr/local/bin/aerolab
```

That's it, now the command `aerolab` is available in your shell globally wherever you are.

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

#### Shell completion

To install shell completion, do one (or both) of:

```
aerolab completion zsh
aerolab completion bash
```

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

### Node Expander

For commands accepting a list of nodes, the list is a comma-separated list of:
* `ALL` - all nodes
* `-X` - negative number - exclude node
* `X` - positive number - include node
* `X-Y` - range of nodes to include

For example:
* `ALL,-5` - all nodes except for node 5
* `1-10,-5,12` - nodes 1-10, except node 5, and also node 12
