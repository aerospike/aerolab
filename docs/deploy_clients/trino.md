[Docs home](../../README.md)

# Deploy a Trino server


Launch a [Trino](https://trino.io/) server and shell, and query your Aerospike cluster with SQL.

## Get the Aerospike cluster IPs

```bash
aerolab cluster list
```

## Create the Trino client machine

Use `172.17.0.3:3000` as the Aerospike server seed IP:PORT

```bash
aerolab client create trino -n trino -s 172.17.0.3:3000
```

## Change which Aerospike cluster the Trino server communicates with

Use `172.17.0.4:3000` as seed IP:PORT

```bash
aerolab client configure trino -n trino -s 172.17.0.4:3000
```

## Attach to the Trino shell
In this example we'll connect the shell to the Aerospike cluster's test
namespace.

```bash
aerolab client attach trino -n trino -m test
```

## Attach to the Trino shell - the long way
This example demonstrates passing the Trino shell command to the Trino client
machine.
```bash
aerolab client attach -n trino -- su - trino -c "bash ./trino --server 127.0.0.1:8080 --catalog aerospike --schema test"
```
