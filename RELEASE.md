### Upgrading

If using AWS or GCP backends, extra steps are required to migrate the firewalls from AeroLab version 6.0 or below to a new system.

[Follow this manual for upgrade instructions](https://github.com/aerospike/aerolab/blob/v7.1.0/docs/upgrade-to-610.md)

NOTE: AeroLab 7.0.0 implements an instance expiry system. By default your instances will terminate after 30 hours. To modify this behaviour, create clusters with `--aws-expires TIME` or `--gcp-expires`. For example `--aws-expires 50h`. To disable expiry, set to `0`.

### Documentation and changelog
See [the documentation](https://github.com/aerospike/aerolab/blob/v7.1.0/README.md) for full installation and usage instructions.

[Changelog](https://github.com/aerospike/aerolab/blob/v7.1.0/CHANGELOG.md#7.1.0)

### Download aerolab from Assets below

Head to the releases page and download one of the installers, depending on where you are intending to execute aerolab command itself.

Note that aerolab will still be able to deploy Aerospike on both arm and x64 architectures, regardless of which aerolab binary you are using.

AeroLab is currently compiled and packaged for amd64 and arm64 as below:
* MacOS as an installer and compressed binaries
* Linux as zip, deb and rpm
* Windows as zip executables

#### Install - MacOS and Linux

It is advisable to use the provided installer files for your distro. Upon download, run the installation and `aerolab` command will become available.

Alternatively, manual installation can be performed by downloading the relevant `zip` file, unpacking it, and then moving the unpacked `aerolab` binary to `/usr/local/bin/` or `/usr/bin/`. Remember to `chmod +x` the binary too.

#### Install - Windows users

Download the zip file, unpack it to a directory (for example `c:\apps\aerolab.exe`) and make an alias in Powershell with `New-Alias -Name aerolab -Value c:\apps\aerolab.exe`.
