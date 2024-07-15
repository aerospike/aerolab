### Upgrading

If using AWS or GCP backends, extra steps are required to migrate the firewalls from AeroLab version 6.0 or below to a new system. [Follow this manual for upgrade instructions](https://github.com/aerospike/aerolab/blob/master/docs/upgrade-to-610.md)

### Documentation and changelog
See [the documentation](https://github.com/aerospike/aerolab/blob/master/README.md) for full installation and usage instructions.

[Changelog](https://github.com/aerospike/aerolab/tree/master/CHANGELOG/)

### Download aerolab from Assets below

Download one of the installers from the releases page, depending on where you are intending to execute aerolab command itself. Aerolab will still be able to deploy Aerospike on both arm and x64 architectures, regardless of which aerolab binary you are using.

AeroLab is currently compiled and packaged for amd64 and arm64 as below:
* MacOS as a multiarch installer and compressed binaries
* Linux as zip, deb and rpm
* Windows as zipped executables

#### Install - MacOS and Linux

It is advisable to use the provided installer files for your distro. Upon download, run the installation and `aerolab` command will become available.

Alternatively, manual installation can be performed by downloading the relevant `zip` file, unpacking it, and then moving the unpacked `aerolab` binary to `/usr/local/bin/` or `/usr/bin/`. Remember to `chmod +x` the binary.

#### Install - Windows users

Download the zip file, unpack it and run it from `Explorer` by double-clicking on it. AeroLab will install itself and become available in `PowerShell` as the `aerolab` command. You may need to close and reopen PowerShell for the changes to take effect.

Alternatively, the binary itself may be called straight from `PowerShell` without first installing.
