# REST API

AeroLab has basic support for a simplified implementation of REST APIs, should these be preferred over the command line interface.

## Usage

### Start Rest API

```
$ aerolab rest-api
2022/10/19 15:42:24 Listening on 127.0.0.1:3030...
```

A custom IP:PORT can be specified instead of the defaults:

```
$ aerolab rest-api -l 0.0.0.0:3000
2022/10/19 16:56:23 Listening on 0.0.0.0:3000...
```

Authentication and TLS are not currently supported. Should this be required, please use a webserver in front of aerolab.

### API usage

AeroLab APIs require a path representing the cli commands. Once a command is selected via the URI, parameters are provided in the payload in JSON format.

The API is fully explorable and well documented within `aerolab` itself.

While all the command-line features are supported by the API in full, the response message are also in the same format as would be provided by the CLI (plain-text).

If an error occurs during the execution of a given command, either `500 InternalServerError` or `400 BadRequest` HTTP response code will be returned to the caller. On successful execution, `200 OK` will be returned.

Try the following to explore the APIs and available options.

### Help pages

#### Get a list of top-level commands:

```
$ curl -X POST http://127.0.0.1:3030/help

Command     | Description
------------+--------------------------------------------------
/config     | Show or change aerolab configuration
/cluster    | Create and manage Aerospike clusters and nodes
/aerospike  | Aerospike daemon controls
/client     | Create and manage Client machine groups
/attach     | Attach to a node and run a command
/net        | Firewall and latency simulation
/conf       | Manage Aerospike configuration on running nodes
/tls        | Create or copy TLS certificates
/data       | Insert/delete Aerospike data
/template   | Manage or delete template images
/installer  | List or download Aerospike installer versions
/logs       | show or download logs
/files      | Upload/Download files to/from clients or clusters
/xdr        | Mange clusters' xdr configuration
/roster     | Show or apply strong-consistency rosters
/version    | Print AeroLab version
/completion | Install shell completion scripts
/rest-api   | Launch HTTP rest API
/help       | Print help
/quit       | Exit aerolab rest service
```

#### Get a list of subcommands within the cluster command

```
$ curl -X POST http://127.0.0.1:3030/cluster/help

Command          | Description
-----------------+----------------------------------
/cluster/create  | Create a new cluster
/cluster/list    | List clusters
/cluster/start   | Start cluster
/cluster/stop    | Stop cluster
/cluster/grow    | Add nodes to cluster
/cluster/destroy | Destroy cluster
/cluster/add     | Add features to clusters, ex: ams
```

#### Get a list of available JSON payload parameters for calling the destroy subcommand

Since `/cluster/destroy` subcommand is an actual execution command (will execute an action) and does not have any subcommands of it's own, executing help here will print both the accepted JSON payload (with default parameters already filled in), and a table explaining what each parameter represents.

```
$ curl -X POST http://127.0.0.1:3030/cluster/destroy/help

=== JSON payload with default values ===
{
  "ClusterName": "mydc",
  "Nodes": "",
  "Docker": {
    "Force": false
  }
}

=== Payload Parameter descriptions ===
Key          | Kind    | Description
-------------+---------+---------------------------------------------------------------
ClusterName  | string  | Cluster names, comma separated OR 'all' to affect all clusters
Nodes        | string  | Nodes list, comma separated. Empty=ALL
Docker.Force | boolean | force stop before destroy
```

### Examples

#### Create a new 4-node cluster, naming it bob

```
curl -X POST http://127.0.0.1:3030/cluster/create -d '{"ClusterName":"bob","NodeCount":4}'
```

#### List root directory contents

```
curl -X POST http://127.0.0.1:3030/attach/shell -d '{"ClusterName":"bob","Tail":["ls","/"]}'
```

#### List clusters, standard table format

```
curl -X POST http://127.0.0.1:3030/cluster/list
```

#### List clusters, provide json output

```
curl -X POST http://127.0.0.1:3030/cluster/list -d '{"Json":true}'
```

#### Destroy the cluster

```
curl -X POST http://127.0.0.1:3030/cluster/destroy -d '{"ClusterName":"bob","Docker":{"Force":true}}'
```
