# AeroLab

Lab tool to quickly spin up Aerospike clusters on docker or in AWS).

## Releases: binaries

Go to the [releases page](https://github.com/citrusleaf/aerolab/releases) and download the required single binary.

Operating System | Binary | Notes
--- | --- | ---
MacOS | aerolab-macos | Native MacOS binary, compiled for x86_64
Linux | aerolab-linux | Native package for linux (all x86_64 distros)
Windows | aerolab-linux | Install and start using WSL2 on Windows

## Supported backends

* Docker
  * on MacOS
  * on Linux
  * on Windows
* AWS

## Routing to the containers

Containers cannot be accessed directly by their IPs in certain docker installations (specifically on Mac and Windows). To work around this, a tunnel can be created to a docker container as a one-off job. Once done, containers on aerolab can be accessed directly (e.g. if your node is on 172.17.0.3 port 3000, you can directly seed from that node on the desktop). This is particularly important when starting multi-node clusters, as client code on the desktop must be able to connect to all nodes.

Follow [this manual](tunnel-container-openvpn/README.md) to setup tunneling for direct container access.

IPs of aerolab containers can be seen by running `aerolab cluster-list`

## Usage instructions

* [Getting Started](docs/GETTING_STARTED.md)
* [Aerolab help commands](docs/USING_HELP.md)
* [Basic Usage](docs/basic/README.md)
* [Advanced Examples](docs/advanced/README.md)
* [AWS HowTo](docs/aws/README.md)
* [Useful scripts](scripts/README.md)
  * [Deploy Strong Consistency cluster](scripts/STRONG.md)
  * [Deploy LDAP server](scripts/aerolab-ldap/README.md)
  * [Build a cluster with LDAP and TLS](scripts/aerolab-buildenv/README.md)

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for version changes

## Version

See [VERSION.md](VERSION.md) for latest stable version number
