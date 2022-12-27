# AeroLab

Lab tool for spinning up Aerospike development and testing clusters on Docker or in AWS.

## Releases

Go to the [releases page](https://github.com/aerospike/aerolab/releases) and download the correct binary for your operating system and processor.

Operating System | Package | Notes
--- | --- | ---
macOS | `aerolab-macos-*` | Native macOS binary, compiled for x86_64 and M series ARM chips
Linux | `aerolab-linux-*` | Native package for Linux (all x86_64 and ARM64 distros)
Windows | `aerolab-linux-*` | Install and start using WSL2 on Windows


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

## Usage instructions

* [Getting Started](docs/GETTING_STARTED.md)
* [AeroLab Help Commands](docs/USING_HELP.md)
* [Usage Examples](docs/usage/README.md)
* [Setting up Aerospike clusters on AWS with AeroLab](docs/aws/README.md)
* [Deploy Clients with AeroLab](docs/usage/CLIENTS.md)
  * [Deploy a VS Code Client Machine](docs/usage/vscode.md) - Launch a [VS Code](https://code.visualstudio.com/) IDE in a browser, complete with Java, Go, Python and C# language clients, and code against your Aerospike cluster
  * [Deploy a Trino server](docs/usage/trino.md) - Launch a [Trino](https://trino.io/) server and shell, and query your Aerospike cluster with SQL
* [Useful Scripts](scripts/README.md)
  * [Create new certificates](scripts/CERTS.md)
  * [Deploy an LDAP server](scripts/aerolab-ldap/README.md)
  * [Build a cluster with LDAP and TLS](scripts/aerolab-buildenv/README.md)
  * [Quick build client - Python](scripts/aerolab-pythonclient/README.md)
  * [Quick build client - Golang](scripts/aerolab-goclient/README.md)

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for version changes

## Version

See [VERSION.md](VERSION.md) for latest stable version number
