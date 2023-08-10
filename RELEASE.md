### Upgrading

If using AWS or GCP backends, extra steps are required to migrate the firewalls from AeroLab version 6.0 or below to a new system.

[Follow this manual for upgrade instructions](https://github.com/aerospike/aerolab/blob/v7.0.0/docs/upgrade-to-610.md)

NOTE: AeroLab 7.0.0 implements an instance expiry system. By default your instances will terminate after 30 hours. To modify this behaviour, create clusters with `--expires TIME`. For example `--expires 50h`. To disable expiry, use `--expires 0`.

### Documentation and changelog
See [the documentation](https://github.com/aerospike/aerolab/blob/v7.0.0/README.md) for full installation and usage instructions.

[Changelog](https://github.com/aerospike/aerolab/blob/v7.0.0/CHANGELOG.md#7.0.0)

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
