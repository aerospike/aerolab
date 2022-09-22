# Using built-in help in aerolab

The help features of aerolab are written in a way that using and exploring aerolab should be simple and easy

## Command list

### Top-level commands

Get help on top-level commands

```bash
% aerolab help
Usage:
  aerolab [OPTIONS] <command>

Available commands:
  config     Show or change aerolab configuration
  cluster    Create and manage Aerospike clusters and nodes
  aerospike  Aerospike daemon controls
  attach     Attach to a node and run a command
  net        Firewall and latency simulation
  conf       Manage Aerospike configuration on running nodes
  tls        Create or copy TLS certificates
  data       Insert/delete Aerospike data
  template   Manage or delete template images
  installer  List or download Aerospike installer versions
  logs       show or download logs
  files      Upload/Download files to/from clients or clusters
  xdr        Manage clusters' xdr configuration
  roster     Show or apply strong-consistency rosters
  version    Print AeroLab version
  help       Print help
```

### Sub-commands

Get help on subcommands of each top-level command

```bash
% aerolab cluster help
Usage:
  aerolab [OPTIONS] cluster [command]

Available commands:
  create   Create a new cluster
  list     List clusters
  start    Start cluster
  stop     Stop cluster
  grow     Add nodes to cluster
  destroy  Destroy cluster
  help     Print help
```

## Command-specific help

Get help on a specific command

```bash
% aerolab cluster stop help
Usage:
  aerolab [OPTIONS] cluster stop [stop-OPTIONS] [help]

[stop command options]
      -n, --name=  Cluster names, comma separated OR 'all' to affect all clusters (default: mydc)
      -l, --nodes= Nodes list, comma separated. Empty=ALL

Available commands:
  help  Print help
```

## Command-specific help mid-way through typing the command

Get help on a specific command half-way through writing it, without executing the command by appending `help` as the last parameter.

```bash
% aerolab cluster create -n test -c 4 help
Usage:
  aerolab [OPTIONS] cluster create [create-OPTIONS] [help]

[create command options]
      -n, --name=                     Cluster name (default: mydc)
      -c, --count=                    Number of nodes (default: 1)
      -o, --customconf=               Custom config file path to install
      -f, --featurefile=              Features file to install
...
```
