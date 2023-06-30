
# AeroLab

AeroLab is a tool that creates Aerospike development and testing clusters in Docker or on AWS, streamlining efforts to test cluster configuration options, upgrade procedures, and client
applications in a controlled development environment.

**NOTE:** AeroLab is intended for local development and testing environments. It is not recommended for production operations. 

## Releases

The [releases page](https://github.com/aerospike/aerolab/releases) contains links to current installer
packages for all the supported backends.

Operating System | Package | Notes
--- | --- | ---
macOS | `aerolab-macos-*` | Native macOS binary, compiled for x86_64 and M series ARM chips
Linux | `aerolab-linux-*` | Native package for Linux (all x86_64 and ARM64 distros)
Windows | `aerolab-linux-*` | Install and start using [WSL2](https://learn.microsoft.com/en-us/windows/wsl/about) on Windows


## Supported backends

* Docker
  * on macOS
  * on Linux
  * on Windows
* Podman (with the following command to enable: `alias docker=podman`)
* AWS
* GCP

## Routing to the containers using other Docker solutions

Containers cannot be accessed directly by their IPs in certain Docker installations (specifically on macOS
and Windows). To work around this, you can create a tunnel to a Docker container. Once a tunnel container is set up,
you can access AeroLab containers directly. For example, if your Aerospike node is on 172.17.0.3 port 3000,
you can directly seed from that node on the desktop. This is particularly important when starting multi-node
clusters, because client code on the desktop must be able to connect to all nodes.

See the tunnel container [setup instructions](docs/tunnel-container-setup.md) for more information about
setting up tunneling for direct container access.

You can see the IP addresses of running AeroLab containers with the `aerolab cluster-list` command.

## Documentation

* [Getting started](docs/GETTING_STARTED.md)
* [Help commands](docs/usage/help.md)
* [GCP](docs/gcp-setup.md)
  * [Partitioner](docs/partitioner/partition-disks.md)
  * [Advanced - GCP Firewall Rules](docs/gcp-firewall.md)
* [AWS](docs/aws-setup.md)
  * [Partitioner](docs/partitioner/partition-disks.md)
  * [Advanced - Custom VPC](docs/vpc.md)
* [Usage examples](docs/usage/index.md)
  * [Basic](docs/usage/basic/index.md)
  * [Advanced](docs/usage/advanced/index.md)
  * [Full Stack](docs/usage/full-stack/index.md)
* [Deploying clients](docs/deploy_clients/index.md)
  * [Elastic Search](docs/deploy_clients/elasticsearch.md)
  * [Rest Gateway](docs/deploy_clients/restgw.md)
  * [Trino](docs/deploy_clients/trino.md)
  * [VSCode](docs/deploy_clients/vscode.md)
  * [AMS monitoring stack](docs/usage/monitoring/ams.md)
  * [Tools and Asbench](docs/usage/full-stack/index.md)
* [REST API](docs/rest-api.md)
* [Utility scripts](docs/utility_scripts/index.md)

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for version changes

## Version

See [VERSION.md](VERSION.md) for latest stable version number
