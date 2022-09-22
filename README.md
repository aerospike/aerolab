# AeroLab

Lab tool for spinning up Aerospike development and testing clusters on Docker or in AWS.

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

## Community-supported docker backend on MacOS (Intel/M1)

A community-supported version of Docker on Mac exists, with full Intel and M1 chip compatibility. This can be used with AeroLab without any issues.

See [https://github.com/aerospike-community/docker-amd64-mac-m1](https://github.com/aerospike-community/docker-amd64-mac-m1) for installation instructions.

The above Docker installation automatically injects routes into the MacOS routing table, allowing for direct communication between the host and the containers.

## Routing to the containers using other docker solutions

Containers cannot be accessed directly by their IPs in certain docker installations (specifically on Mac and Windows). To work around this, a tunnel can be created to a docker container as a one-off job. Once done, containers on aerolab can be accessed directly (e.g. if your node is on 172.17.0.3 port 3000, you can directly seed from that node on the desktop). This is particularly important when starting multi-node clusters, as client code on the desktop must be able to connect to all nodes.

Follow [this manual](tunnel-container-openvpn/README.md) to setup tunneling for direct container access.

IPs of aerolab containers can be seen by running `aerolab cluster-list`

## Usage instructions

* [Getting Started](docs/GETTING_STARTED.md)
* [Aerolab help commands](docs/USING_HELP.md)
* [Usage Examples](docs/usage/README.md)
* [AWS HowTo](docs/aws/README.md)
* [Useful scripts](scripts/README.md)
  * [Create new certificates](scripts/CERTS.md)
  * [Deploy LDAP server](scripts/aerolab-ldap/README.md)
  * [Build a cluster with LDAP and TLS](scripts/aerolab-buildenv/README.md)
  * [Quick build client - python](scripts/aerolab-pythonclient/README.md)
  * [Quick build client - golang](scripts/aerolab-goclient/README.md)

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for version changes

## Version

See [VERSION.md](VERSION.md) for latest stable version number
