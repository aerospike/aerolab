# Early proposal for command line adjustment

## Help

```
% ./aerolab help
AeroLab - Deploy and manage development Aerospike clusters and clients

Usage:
  aerolab [command]

Available Commands:
  cluster     Aerospike cluster management
  completion  Generate the autocompletion script for the specified shell
  config      Adjust or display persistent AeroLab configuration
  help        Help about any command
  lossDelay   Implement packet loss or latencies
  net         Network firewall management
  xdr         XDR autoconfiguration

Flags:
      --config string   config file (default "/Users/rglonek/.aerolab.yaml")
  -h, --help            help for aerolab

Use "aerolab [command] --help" for more information about a command.
```

## Cluster

```
% ./aerolab cluster help
Manage aerospike cluster machines

Usage:
  aerolab cluster [command]

Available Commands:
  create      Create a new cluster
  destroy     Destroy a cluster
  start       Start all cluster machines
  stop        Stop all cluster machines
  grow        Add nodes to the cluster

Flags:
  -h, --help   help for cluster

Global Flags:
      --config string   config file (default "/Users/rglonek/.aerolab.yaml")

Use "aerolab cluster [command] --help" for more information about a command.
```

## Cluster create

```
% ./aerolab cluster create --help
Create a cluster

Usage:
  aerolab cluster create [flags]

Flags:
  -v, --aerospike-version string   Aerospike version (default "latest")
  -c, --count string               Number of nodes (default "1")
  -h, --help                       help for create
  -m, --mode string                Heartbeat mode; mesh|multicast; default: don't touch
  -n, --name string                Cluster name (default "mydc")
  [...]

Global Flags:
      --config string   config file (default "/Users/rglonek/.aerolab.yaml")
```

## Example

```
$ aerolab config backend --type docker
OK
$ aerolab config defaults --features-file /path/to/features.conf
OK
$ aerolab cluster create -n bob -c 2
$ aerolab node attach -n bob -l 1
...
$ aerolab cluster destroy -n bob -f

$ aerolab config backend --type aws
OK
```

## Notes

The multi-layered command structure (`aerolab {command} {subcommand} {-flags}`) will allow for easy expansion and future-proof the command line. Current feature set is very rich and the commands as well as help pages are becoming very crowded. Future addition of easy deployment of multiple clients for example will make managing current command line structure very difficult. The above solved this.

Current approach is built towards docker, with aws bolted on top. Hence, currently, to use aerolab with aws, certain parameters must be provided as environment variables, certain parameters for aws are irrelevant (docker-only) and certain features don't work in one backend or another.

The proposed solution would require to set (and allow to easily change) the backend, thus allowing for command line parameters (for example for `aerolab cluster create`) to only show the relevant switches for the given backend. This will allow for certain shared switches, and certain switches to be backend-specific, allowing easy separation between backends, and allow for easy addition of other backends in the future.

## Current command line

Current command-line is flat structure:

```
aerolab make-cluster
aerolab cluster-destroy
aerolab node-attach
etc
```

Currently backend is chosen by specifying a command line parameter to each command.
