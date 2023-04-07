# AeroLab

Lab tool for spinning up Aerospike development and testing clusters on Docker or in AWS.

## Releases

Go to the [releases page](https://github.com/aerospike/aerolab/releases) and download the correct binary for your operating system and processor.

Operating System | Package | Notes
--- | --- | ---
macOS | `aerolab-macos-*` | Native macOS binary, compiled for x86_64 and M series ARM chips
Linux | `aerolab-linux-*` | Native package for Linux (all x86_64 and ARM64 distros)
Windows | `aerolab-linux-*` | Install and start using WSL2 on Windows

## Usage instructions

[Official documentation can be found here](https://docs.aerospike.com/tools/aerolab)

## Supported backends

* Docker
  * on macOS
  * on Linux
  * on Windows
* AWS

## Routing to the containers using other Docker solutions

Containers cannot be accessed directly by their IPs in certain Docker installations (specifically on macOS and Windows). To work around this, a tunnel can be created to a Docker container as a one-off job. Once done, containers on AeroLab can be accessed directly (e.g. if your node is on 172.17.0.3 port 3000, you can directly seed from that node on the desktop). This is particularly important when starting multi-node clusters, as client code on the desktop must be able to connect to all nodes.

Follow [this manual](tunnel-container-openvpn/README.md) to set up tunneling for direct container access.

IPs of AeroLab containers can be seen by running `aerolab cluster-list`

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for version changes

## Version

See [VERSION.md](VERSION.md) for latest stable version number
