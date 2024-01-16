
# AeroLab

AeroLab is a tool that creates Aerospike development and testing clusters in Docker or on AWS, streamlining efforts to test cluster configuration options, upgrade procedures, and client applications in a controlled development environment.

**NOTE:** AeroLab is intended for local development and testing environments. It is not recommended for production operations. 

## Upgrading

[See this document on upgrading from versions 6.0- to 6.1+](docs/upgrade-to-610.md)

NOTE: AeroLab 7.0.0 implements an instance expiry system. By default your instances will terminate after 30 hours. To modify this behaviour, create clusters with `--aws-expires TIME` or `--gcp-expires`. For example `--aws-expires 50h`. To disable expiry, set to `0`.

## Releases

The [releases page](https://github.com/aerospike/aerolab/releases) contains links to current installer
packages for all the supported backends.

Operating System | Package | Notes
--- | --- | ---
macOS | `aerolab-macos-*` | Native macOS binary, compiled for x86_64 and M series ARM chips
Linux | `aerolab-linux-*` | Native package for Linux (all x86_64 and ARM64 distros)
Windows | `aerolab-windows-*` | Native Windows executable. Unzip and run from explorer to install.

## Manual build

See [this document](docs/building.md) for manually building AeroLab (not recommended).

## Supported backends

* Docker Native and Docker Desktop
  * on macOS
  * on Linux
  * on Windows
* Podman Desktop
   * Install Podman Desktop
   * Enable Docker Compatibility mode in Podman Desktop
   * Use `docker` backend in aerolab: `aerolab config backend -t docker`
   * Install full official `docker-cli`
     * Example MacOS: `brew install docker`
     * Example Ubuntu: follow the [official documentation](https://docs.docker.com/engine/install/ubuntu/), but instead of installing the full docker engine, just install `docker-ce-cli` package
* Podman Native on Linux
  * Follow [podman documentation](https://podman.io/docs/installation) to install podman.
  * To install the full docker cli tool, follow the [official documentation](https://docs.docker.com/engine/install/ubuntu/), but instead of installing the full docker engine, just install `docker-ce-cli` package.
  * Enable podman service with: `sudo systemctl enable --now podman.service podman.socket && sudo touch /etc/containers/nodocker`.
  * Enable the docker repositories: `sudo vi /etc/containers/registries.conf` and ensure docker is listed on this line: `unqualified-search-registries = ["docker.io","localhost"]`.
  * Use `docker` backend in aerolab
* [AWS](docs/aws-setup.md)
* [GCP](docs/gcp-setup.md)

## Routing to the containers using Docker Desktop

Containers on Docker Desktop cannot be accessed directly by their IPs. For this purpose, AeroLab will automatically attempt to map host ports to container ports.

The containers can then be accessed using IP `127.0.0.1` and the port shown under `aerolab inventory list`. Aerospike clusters created using this method can be seeded and connected to directly from the desktop computer, using the `services-alternate` option in either Aerospike tools or in the client libraries.

To disable this functionality and prevent AeroLab from modifying Aerospike configuration files to that effect, create clusters with the `--no-autoexpose` switch.

An alternative method of access exists on MacOS and Linux, if using `--no-autoexpose`. See the tunnel container [setup instructions](docs/tunnel-container-setup.md) for more information about setting up tunneling for direct container access.

## Documentation

* [Getting started](docs/GETTING_STARTED.md)
* [Help commands](docs/usage/help.md)
* [GCP](docs/gcp-setup.md)
  * [Spot instance support](docs/gcp-spot.md)
  * [Expiry system](docs/expiries.md)
  * [Extra Volume Support](docs/efs.md)
  * [Partitioner](docs/partitioner/partition-disks.md)
  * [Advanced - GCP Firewall Rules](docs/gcp-firewall.md)
* [AWS](docs/aws-setup.md)
  * [Spot instance support](docs/aws-spot.md)
  * [EFS Support](docs/efs.md)
  * [Expiry system](docs/expiries.md)
  * [Partitioner](docs/partitioner/partition-disks.md)
  * [Advanced - AWS Firewall Rules](docs/aws-firewall.md)
  * [Advanced - Custom VPC](docs/vpc.md)
* [Usage examples](docs/usage/index.md)
  * [Basic](docs/usage/basic/index.md)
  * [Advanced](docs/usage/advanced/index.md)
  * [Full Stack](docs/usage/full-stack/index.md)
* [DirEnv - different aerolab configuration per directory](docs/direnv.md)
* [AGI - graphing aerospike statistics from logs](docs/agi/README.md)
* [Deploying clients](docs/deploy_clients/index.md)
  * [Elastic Search](docs/deploy_clients/elasticsearch.md)
  * [Rest Gateway](docs/deploy_clients/restgw.md)
  * [Trino](docs/deploy_clients/trino.md)
  * [VSCode](docs/deploy_clients/vscode.md)
  * [AMS monitoring stack](docs/usage/monitoring/ams.md)
  * [Tools and Asbench](docs/usage/full-stack/index.md)
* [REST API](docs/rest-api.md)
* [Utility scripts](docs/utility_scripts/index.md)
* [Volume usage examples](docs/volume-examples.md)

## Changelog

See [CHANGELOG.md](CHANGELOG.md) for version changes

## Version

See [VERSION.md](VERSION.md) for latest stable version number

## Disabling console colors

Aerolab list commands by default use neutral coloring to compress and present the listing tables of all items. Coloring can be disabled by exporting one of these environment variables:
* `export NO_COLOR=1`
* `export CLICOLOR=0`

The following methods work:
1. just for this command: `CLICOLOR=0 aerolab cluster list`
2. for this terminal session: `export CLICOLOR=0`
3. forever: add one of the `export` commands from above your your `.zshrc` or `.bashrc` file
