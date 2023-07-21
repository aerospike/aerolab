### Upgrading

If using AWS or GCP backends, extra steps are required to migrate the firewalls to a new system.

[Follow this manual for upgrade instructions](https://github.com/aerospike/aerolab/blob/v7.0.0/docs/upgrade-to-610.md)

### Documentation and changelog
See [the documentation](https://github.com/aerospike/aerolab/blob/v7.0.0/README.md) for full installation and usage instructions.

[Changelog](https://github.com/aerospike/aerolab/blob/v7.0.0/CHANGELOG.md#7.0.0)

### Latest changes:
* Add option for auto-expiring clusters in GCP and AWS.
* Add option to map and expose ports in Docker backend on a 1:1 pairing (eg 3100 node 1, 3101 node 2 etc) - so not workarounds are needed to access Aerospike clusters on Docker Desktop from the Desktop.
* AeroLab Support for deploying Amazon 2023 server >= 6.4.
* AeroLab Support for deploying Debian 12 server >= 6.4.
* Add MacOS packaging and signing to makefile to move fully away from bash scripts.
* Add deprecation warning around client machines and how they are handled in 8.0.
* Message INFO at end of cluster create pointing user at AMS documentation informing them ams exists.
* GCP move some values from labels to custom metadata
* GCP support setting custom metadata values
* Check Aerospike-AWS-Secrets-Manager support adding

### Download aerolab from Assets below

Head to the releases page and download one of the installers, depending on where you are intending to execute aerolab command itself.

Note that aerolab will still be able to deploy Aerospike on both arm and x64 architectures, regardless of which aerolab binary you are using.

Operating System | CPU | File | Comments
--- | --- | --- | ---
MacOS | ALL | `aerolab-macos-VERSION.pkg` | multi-arch AeroLab installer for MacOS
MacOS | arm | `aerolab-macos-arm64-VERSION.zip` | single executable binary in a zip file
MacOS | Intel/AMD | `aerolab-macos-amd64-VERSION.zip` | single executable binary in a zip file
Linux (generic) | arm | `aerolab-linux-arm64-VERSION.zip` | single executable binary in a zip file
Linux (generic) | Intel/AMD | `aerolab-linux-amd64-VERSION.zip` | single executable binary in a zip file
Linux (centos) | arm | `aerolab-linux-arm64-VERSION.rpm` | RPM for installing on centos/rhel-based distros
Linux (centos) | Intel/AMD | `aerolab-linux-x86_64-VERSION.rpm` | RPM for installing on centos/rhel-based distros
Linux (ubuntu) | arm | `aerolab-linux-arm64-VERSION.deb` | DEB for installing on ubuntu/debian-based distros
Linux (ubuntu) | Intel/AMD | `aerolab-linux-amd64-VERSION.deb` | DEB for installing on ubuntu/debian-based distros

#### Install

It is advisable to use the provided installer files for your distro. Upon download, run the installation and `aerolab` command will become available.

Alternatively, manual installation can be performed by downloading the relevant `zip` file, unpacking it, and then moving the unpacked `aerolab` binary to `/usr/local/bin/` or `/usr/bin/`. Remember to `chmod +x` the binary too.
