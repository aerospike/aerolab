### Upgrading

If using AWS or GCP backends, extra steps are required to migrate the firewalls to a new system.

[Follow this manual for upgrade instructions](https://github.com/aerospike/aerolab/blob/v6.2.0/docs/upgrade-to-610.md)

### Documentation and changelog
See [the documentation](https://github.com/aerospike/aerolab/blob/v6.2.0/README.md) for full installation and usage instructions.

[Changelog](https://github.com/aerospike/aerolab/blob/v6.2.0/CHANGELOG.md#6.2.0)

### Latest changes:
* Add option to filter instance types by name.
* Add pricing information to `inventory instance-types` command.
* Sort options for `inventory instance-types` command.
* Add a 24-hour cache of `inventory instance-types` to allow for quick lookup.
* Add display of price information on cluster and client creation.
* Add option to only display price (without actually creating clusters or clients) using `--price` in the `create` commands.
* Add `nodes count` multiplier to `inventory instance-types` to allow for easy cost visualization.
* Track instance cost in instance tags/labels.
* Show instance running costs in `list` views.
* Parallelize the following commands (with thread limits):
  * `aerospike start/stop/restart/status`
  * `roster show/apply`
  * `files upload/download/sync`
  * `tls generate/copy`
  * `conf fix-mesh/rack-id`
  * `cluster create/start/partition */add exporter`
  * `xdr connect/create-clusters`
  * `client create base/none/tools`
  * `client configure tools`
* Some config parsing bugfixes for handling of accidentally saved `aerolab config defaults` values.
* Allow specifying not to configure TLS for `mesh` in `tls generate` command.
* Allow specifying multiple disks of same type in `gcp` backend; for example `--disk local-ssd@5` will request 5 local SSDs and `--disk pd-ssd:50@5` will request 5 `pd-ssd` of size 50GB.
* Improve feature key file version checking.
* Allow tagging by `owner` during cluster and client creation. Specify owner to always use with `aerolab config defaults -k '*.Owner' -v owner-name-all-lowercase-no-spaces`.
* Capture outcomes of run commands in internal user telemetry.
* Add telemetry version into output.
* Add `conf adjust` to allow for adjusting aerospike configuration on a deployed cluster on the fly.
* Move to using `Makefiles` for build and linux packaging process.
* Fix handling of extra docker flags in docker backend.

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
