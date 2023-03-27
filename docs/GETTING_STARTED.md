# Getting started

## One-time setup

Follow either the **Docker** or **AWS** manual below, depending on the backend you wish to you. Both backends may be used and AeroLab may be switched between them.

### Docker

Follow the below if using the Docker backend.

#### Install Docker

Use one of the below methods to install Docker:

* Install [Docker Desktop](https://www.docker.com/products/docker-desktop/) on your machine
* Install Docker using [`minikube`](https://minikube.sigs.k8s.io/docs/start/) on your machine

#### Start Docker

Start Docker and ensure it's running by executing `docker version`

#### Configure disk, RAM and CPU resources

If using Docker Desktop, in the Docker tray-icon, go to "Preferences". Configure the required disk, RAM and CPU resources. At least 2 cores and 2 GB of RAM is recommended for a single-node cluster.

### AWS

Follow the below if using the AWS backend.

#### Configure aws-cli

Follow [this manual](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) to install the AWS CLI.

Run `aws configure` to configure basic access to AWS.

### Download AeroLab from the releases page

Head to the releases page and download one of the installers, depending on where you are intending to run AeroLab.

Note that AeroLab will still be able to deploy Aerospike on both ARM64 and x86_64 architectures, regardless of which AeroLab binary you are using.

Operating System | CPU | File | Comments
--- | --- | --- | ---
macOS | ALL | `aerolab-macos.pkg` | multi-arch AeroLab installer for macOS
macOS | M1 or M2 | `aerolab-macos-arm64.zip` | single executable binary in a zip file
macOS | Intel CPU | `aerolab-macos-amd64.zip` | single executable binary in a zip file
Linux (generic) | arm | `aerolab-linux-arm64.zip` | single executable binary in a zip file
Linux (generic) | Intel/AMD | `aerolab-linux-amd64.zip` | single executable binary in a zip file
Linux (centos) | arm | `aerolab-linux-arm64.rpm` | RPM for installing on centos/rhel-based distros
Linux (centos) | Intel/AMD | `aerolab-linux-x86_64.rpm` | RPM for installing on centos/rhel-based distros
Linux (ubuntu) | arm | `aerolab-linux-arm64.deb` | DEB for installing on ubuntu/debian-based distros
Linux (ubuntu) | Intel/AMD | `aerolab-linux-amd64.deb` | DEB for installing on ubuntu/debian-based distros

#### Install

It is adviseable to use the provided installer files. Upon download, run the installation and `aerolab` command will become available.

Alternatively, manual installation can be performed by downloading the relevant `zip` file, unpacking it, and then moving the unpacked `aerolab` binary to `/usr/local/bin/`. Remember to `chmod +x` the binary too.

### First run

#### First run will print help page asking for backend configuration. Do so by following thr printed help page:

```bash
% aerolab

Create a config file and select a backend first using one of:

$ aerolab config backend -t docker [-d /path/to/tmpdir/for-aerolab/to/use]
$ aerolab config backend -t aws [-r region] [-p /custom/path/to/store/ssh/keys/in/] [-d /path/to/tmpdir/for-aerolab/to/use]

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

#### Windows usage on WSL2 - configuring docker desktop

1. Open `Docker Desktop`, navigate to `Settings` and under `General` select `Use the WSL 2 based engine`
2. Under `Resources->ESL Integration`, select the virtual machine(s) you want to give access to docker
3. Stop `WSL` by executing `wsl --shutdown`
4. Restart your `WSL` linux virtual machine
5. From within the virtual machine, execute `docker info`. If the docker command is not found, it will need to be installed. Below manual covers installing on `ubuntu` based image:

```
sudo apt-get update && sudo apt-get install ca-certificates curl gnupg lsb-release
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt install docker-ce-cli
docker info
```

#### Windows usage on WSL2 - limitation of WSL2

Windows `WSL2` has an issue with temporary file access from docker. To work around this, specify a temporary directory that is not `/tmp` when using `aerolab` on windows. This can be any existing directory your user can write to. For example:

```
# docker
aerolab config backend -t docker -d ${HOME}

# or aws
aerolab config backend -t aws -d ${HOME}
```

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
$ aerolab attach shell --name=testme --node=<node-expander-syntax> -- asinfo -v service
```

### Configuration Generator

AeroLab can generate basic `aerospike.conf` files by running: `aerolab conf generate`

#### Node Expander

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

