![Release](https://img.shields.io/github/release/bmatcuk/go-vagrant.svg?branch=master)
[![Build Status](https://travis-ci.com/bmatcuk/go-vagrant.svg?branch=master)](https://travis-ci.com/bmatcuk/go-vagrant)
[![codecov.io](https://img.shields.io/codecov/c/github/bmatcuk/go-vagrant.svg?branch=master)](https://codecov.io/github/bmatcuk/go-vagrant?branch=master)
[![Go Report Card](https://goreportcard.com/badge/github.com/bmatcuk/go-vagrant)](https://goreportcard.com/report/github.com/bmatcuk/go-vagrant)
[![GoDoc](https://godoc.org/github.com/bmatcuk/go-vagrant?status.svg)](https://godoc.org/github.com/bmatcuk/go-vagrant)

# go-vagrant
A golang wrapper around the Vagrant command-line utility.


## Installation
**go-vagrant** can be installed with `go get`:

```bash
go get github.com/bmatcuk/go-vagrant
```

Import it in your code:

```go
import "github.com/bmatcuk/go-vagrant"
```

The package name will be `vagrant`.


## Basic Usage
```go
func NewVagrantClient(vagrantfileDir string) (*VagrantClient, error)
```

First, you'll need to instantiate a VagrantClient object using
`NewVagrantClient`. The function takes one argument: the path to the directory
where the Vagrantfile lives. This instantiation will check that the vagrant
command exists and that the Vagrantfile can be read.

```go
func (*VagrantClient) Action() *CommandObject
CommandObject.Option1 = ...
CommandObject.Option2 = ...

func (*CommandObject) Run() error
func (*CommandObject) Start() error
func (*CommandObject) Wait() error
CommandObject.Output
CommandObject.Error
```

From there, every vagrant action follows the same pattern: call the appropriate
method on the client object to retrieve a command object. The command object
has fields for any optional arguments. Then either call the command's `Run()`
method, or call `Start()` and then `Wait()` on it later. Any output from the
command, including errors, will be fields on the command object. The exact
field names for options and output are listed below in the [Actions] section.

For example:

```go
package main

import "github.com/bmatcuk/go-vagrant"

func main() {
  client, err := vagrant.NewVagrantClient(".")
  if err != nil {
    ...
  }

  upcmd := client.Up()
  upcmd.Verbose = true
  if err := upcmd.Run(); err != nil {
    ...
  }
  if upcmd.Error != nil {
    ...
  }

  // TODO: vagrant VMs are up!
}
```


## Available Actions

### BoxAdd
Adds a box to your local vagrant box collection. Takes a location which is the 
name, url, or path of the box.

```go
func (*VagrantClient) BoxAdd(location string) *BoxAddCommand
```

Options:
* **Clean** (default: `false`) - Clean any temporary download files. This will
  prevent resuming a previously started download.
* **Force** (default: `false`) - Overwrite an existing box if it exists.
* **Name** (default: `""`) - Name of the box. Only has to be set if the box is
  provided as a path or url without metadata.
* **Checksum** (default: `""`) - Checksum for the box.
* **CheckSumType** - Type of the supplied Checksum. Allowed values are
  vagrant.MD5, vagrant.SHA1, vagrant.SHA256.

Response:
* **Error** - Set if an error occurred.


### BoxList
Return a list of available vagrant boxes.

```go
func (*VagrantClient) BoxList() *BoxListCommand
```

Response:
* **Boxes** - an array of `Box` objects. Each Box has a `Name`, `Provider`, and
  `Version`.
* **Error** - Set if an error occurred.


### Destroy
Stop and delete all traces of the vagrant machines.

```go
func (*VagrantClient) Destroy() *DestroyCommand
```

Options:
* **Force** (default: `true`) - Destroy without confirmation. Defaults to true
  because, when it's false, vagrant will try to ask for confirmation but
  complain that there's no attached TTY.
* **Parallel** (default: `false`) - Enable or disable parallelism if provider
  supports it (automatically enables Force).
* **MachineName** (default: `""`) - The name or ID of a vagrant VM to destroy.
  If unspecified (default), all VMs in the current directory will be destroyed.

Response:
* **Error** - Set if an error occurred.


### GlobalStatus
Get the status of all vagrant machines.

```go
func (*VagrantClient) GlobalStatus() *GlobalStatusCommand
```

Options:
* **Prune** (default: `false`) - Remove invalid entries

Response:
* **Error** - Set if an error occurred.
* **Status** - A map of vagrant machine IDs (ex: 1a2b3c4d) to GlobalStatus
  objects which describe the name, state, and directory in which the machine
  was created.


### Halt
Stops the vagrant machine.

```go
func (*VagrantClient) Halt() *HaltCommand
```

Options:
* **Force** (default: `false`) - Force shutdown (equivalent to pulling the
  power of the VM)
* **MachineName** (default: `""`) - The name or ID of a vagrant VM to halt.
  If unspecified (default), all VMs in the current directory will be halted.

Response:
* **Error** - Set if an error occurred.


### Port
Get information about guest port forwarded mappings.

```go
func (*VagrantClient) Port() *PortCommand
```

Options:
* **MachineName** (default: `""`) - Specify the vagrant machine you are
  interested in. If your Vagrantfile only brings up one machine, you do not
  need to specify this. However, if your Vagrantfile brings up multiple
  machines, you *must* specify this! For some reason, this is the only vagrant
  command that cannot handle multiple machines.

Response:
* **ForwardedPorts** - an array of `ForwardedPort` objects. Each ForwardedPort
  has a `Guest` and a `Host` port, representing a mapping from the host port
  to the guest.
* **Error** - Set if an error occurred.


### Provision
Provision the vagrant machine.

```go
func (*VagrantClient) Provision() *ProvisionCommand
```

Options:
* **Provisioners** (default: `nil`) - Enable only certain provisioners, by type
  or name as an array of strings.
* **MachineName** (default: `""`) - The name or ID of a vagrant VM to provision.
  If unspecified (default), all VMs in the current directory will be
  provisioned.

Response:
* **Error** - Set if an error occurred.


### Reload
Restarts the vagrant machine and loads any new Vagrantfile configuration.

```go
func (*VagrantClient) Reload() *ReloadCommand
```

Options:
* **Provisioning** (default: `DefaultProvisioning`) - By default will only run
  provisioners if they haven't been run already. If set to ForceProvisioning
  then provisioners will be forced to run; if set to DisableProvisioning then
  provisioners will be disabled.
* **Provisioners** (default: `nil`) - Enable only certain provisioners, by type
  or name as an array of strings. Implies ForceProvisioning.
* **MachineName** (default: `""`) - The name or ID of a vagrant VM to reload.
  If unspecified (default), all VMs in the current directory will be reloaded.

Response:
* **Error** - Set if an error occurred.


### Resume
Resume a suspended vagrant machine

```go
func (*VagrantClient) Resume() *ResumeCommand
```

Options:
* **Provisioning** (default: `DefaultProvisioning`) - By default will only run
  provisioners if they haven't been run already. If set to ForceProvisioning
  then provisioners will be forced to run; if set to DisableProvisioning then
  provisioners will be disabled.
* **Provisioners** (default: `nil`) - Enable only certain provisioners, by type
  or name as an array of strings. Implies ForceProvisioning.
* **MachineName** (default: `""`) - The name or ID of a vagrant VM to resume.
  If unspecified (default), all VMs in the current directory will be resumed.

Response:
* **Error** - Set if an error occurred.


### SSHConfig
Get SSH configuration information for the vagrant machine.

```go
func (*VagrantClient) SSHConfig() *SSHConfigCommand
```

Options:
* **Host** (default: `""`) - Name the host for the config
* **MachineName** (default: `""`) - The name or ID of a vagrant VM to retrieve
  the configuration for. If unspecified (default), the configuration of all VMs
  in the current directory will be returned.

Response:
* **Configs** - a map of vagrant machine names to `SSHConfig` objects. Each
  SSHConfig has several fields including Host, User, Port, etc. You can see
  full field list in the [godocs for SSHConfig].


### Status
Get the status of the vagrant machine.

```go
func (*VagrantClient) Status() *StatusCommand
```

Options:
* **MachineName** (default: `""`) - The name or ID of a vagrant VM to retrieve
  the status for. If unspecified (default), the status of all VMs in the
  current directory will be returned.

Response:
* **Status** - a map of vagrant machine names to a string describing the
  status of the VM.
* **Error** - Set if an error occurred.


### Suspend
Suspends the vagrant machine.

```go
func (*VagrantClient) Suspend() *SuspendCommand
```

Options:
* **MachineName** (default: `""`) - The name or ID of a vagrant VM to suspend.
  If unspecified (default), all VMs in the current directory will be suspended.

Response:
* **Error** - Set if an error occurred.


### Up
Starts and provisions the vagrant machine.

```go
func (*VagrantClient) Up() *UpCommand
```

Options:
* **Provisioning** (default: `DefaultProvisioning`) - By default will only run
  provisioners if they haven't been run already. If set to ForceProvisioning
  then provisioners will be forced to run; if set to DisableProvisioning then
  provisioners will be disabled.
* **Provisioners** (default: `nil`) - Enable only certain provisioners, by type
  or name as an array of strings. Implies ForceProvisioning.
* **DestroyOnError** (default: `true`) - Destroy machine if any fatal error
  happens.
* **Parallel** (default: `true`) - Enable or disable parallelism if provider
  supports it.
* **Provider** (default: `""`) - Back the machine with a specific provider.
* **InstallProvider** (default: `true`) - If possible, install the provider if
  it isn't installed.
* **MachineName** (default: `""`) - The name or ID of a vagrant VM to bring up.
  If unspecified (default), all VMs in the current directory will be brought
  up.

Response:
* **VMInfo** - a map of vagrant machine names to `VMInfo` objects. Each VMInfo
  describes the `Name` of the machine and `Provider`.
* **Error** - Set if an error occurred.


### Version
Get the current and latest vagrant version.

```go
func (*VagrantClient) Version() *VersionCommand
```

Response:
* **InstalledVersion** - the version of vagrant installed
* **LatestVersion** - the latest version available
* **Error** - Set if an error occurred.


[Actions]: #available-actions
[godocs for SSHConfig]: https://godoc.org/github.com/bmatcuk/go-vagrant#SSHConfig
