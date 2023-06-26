# Getting started With AeroLab

## One-time setup

Follow either the Docker or AWS instructions below, depending on the backend you use.
You can use both backends at once, and use AeroLab commands on either one.

### Docker instructions

1. Install [Docker Desktop](https://www.docker.com/products/docker-desktop/) on your machine.

2. Start Docker. To make sure it's running, run `docker version` at the command line.

3. Configure disk, RAM,and CPU resources. In the Docker tray-icon, go to `Preferences`. Configure the required disk, RAM and CPU resources. At least 2 cores and 2 GB of RAM is recommended for a single-node cluster.

### AWS

#### configure-aws-cli

Follow [this manual](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) to install the AWS CLI.

Run `aws configure` to configure basic access to AWS.

### Download AeroLab from the releases page

The [releases page](https://github.com/aerospike/aerolab/releases) has links to installers for all the
supported platforms

Note that AeroLab can deploy Aerospike instances on both ARM64 and x86_64 architectures,
regardless of which AeroLab binary you use.

Operating System | CPU | File | Comments
--- | --- | --- | ---
macOS | ALL | `aerolab-macos-VERSION.pkg` | multi-arch AeroLab installer for macOS
macOS | M1 or M2 | `aerolab-macos-arm64-VERSION.zip` | single executable binary in a zip file
macOS | Intel CPU | `aerolab-macos-amd64-VERSION.zip` | single executable binary in a zip file
Linux (generic) | arm | `aerolab-linux-arm64-VERSION.zip` | single executable binary in a zip file
Linux (generic) | Intel/AMD | `aerolab-linux-amd64-VERSION.zip` | single executable binary in a zip file
Linux (centos) | arm | `aerolab-linux-arm64-VERSION.rpm` | RPM for installing on centos/rhel-based distros
Linux (centos) | Intel/AMD | `aerolab-linux-x86_64-VERSION.rpm` | RPM for installing on centos/rhel-based distros
Linux (ubuntu) | arm | `aerolab-linux-arm64-VERSION.deb` | DEB for installing on ubuntu/debian-based distros
Linux (ubuntu) | Intel/AMD | `aerolab-linux-amd64-VERSION.deb` | DEB for installing on ubuntu/debian-based distros

### Install

Installation with the provided installer files is the recommended method. After download, run the executable
and the `aerolab` command will become available.

Alternatively, you can perform a manual installation by downloading the relevant `zip` file, unpacking it,
and then moving the unpacked `aerolab` binary to `/usr/local/bin/`. Remember to run `chmod +x` on the `aerolab`
binary to make it executable.

### First run

Running `aerolab` at the command line outputs a help page asking for backend configuration.

```bash
% aerolab

Create a config file and select a backend first using one of:

$ aerolab config backend -t docker [-d /path/to/tmpdir/for-aerolab/to/use]
$ aerolab config backend -t aws [-r region] [-p /custom/path/to/store/ssh/keys/in/] [-d /path/to/tmpdir/for-aerolab/to/use]
$ aerolab config backend -t gcp -o project-name [-d /path/to/tmpdir/for-aerolab/to/use] [-p /custom/path/to/store/ssh/keys/in/]

Default file path is ${HOME}/.aerolab.conf

To specify a custom configuration file, set the environment variable:
   $ export AEROLAB_CONFIG_FILE=/path/to/file.conf
```

### Configuring defaults

You can change the default configuration with the `aerolab config defaults` command. If you
change the defaults, the new values are used as defaults. You can still change the
configuration at runtime by specifying command line switches.

Command | Description
--- | ---
`aerolab config defaults help` | print help for using the defaults command
`aerolab config defaults` | print all defaults
`aerolab config defaults -o` | print only the defaults that have been adjusted in the config file
`aerolab config defaults -k '*Features*'` | print all defaults containing the word `Features`
`aerolab config defaults -k '*.HeartbeatMode' -v mesh` | adjust `HeartbeatMode` default to `mesh` for all available commands

### Getting started: configuration basics

It's a good idea to configure the basics so you don't have to use command line switches each time.

If you wish to use a custom features file, run the following command:
`aerolab config defaults -k '*FeaturesFilePath' -v /path/to/features.conf`

If using multiple feature file versions, a directory containining those may be specified: `aerolab config defaults -k '*FeaturesFilePath' -v /path/to/features/dir/`

### AeroLab on Windows with WSL2

1. Go to `Docker Desktop` -> `Settings` -> `General`.  Select `Use the WSL 2 based engine`
2. Under `Resources -> WSL Integration`, select the virtual machine(s) you want to give access to Docker
3. Stop `WSL` by executing `wsl --shutdown`
4. Restart your `WSL` Linux virtual machine
5. Fix permissions: `sudo chmod 777 /var/run/docker.sock` - this needs to be done every time Docker is restarted, or alternatively `aerolab` will have to be run with `sudo` each time

### Shell completion

To enable shell completion, run one (or both) of:

```
aerolab completion zsh
aerolab completion bash
```

## Basic usage

### Deploy a cluster called `testme` with 5 nodes
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

### Run `asadm info` command on node 2
```
$ aerolab attach shell --name=testme --node=2 -- asadm -e info
$ aerolab attach asadm --name=testme --node=2 -- -e info
```

### Run `asinfo` on all nodes
```
$ aerolab attach asinfo --name=testme --node=all -- -v service
$ aerolab attach shell --name=testme --node=all -- asinfo -v service
$ aerolab attach shell --name=testme --node=<node-expander-syntax> -- asinfo -v service
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

### Configuration file generator

AeroLab can generate a basic `aerospike.conf` file by running:

````
aerolab conf generate
````
