# Create an Aerospike cluster with Strong Consistency

## Create a cluster, with a custom config and features file
In this example a three node Aerospike Database cluster is created using a
Strong Consistency [template](../../templates/https://github.com/aerospike/aerolab/blob/master/templates/strong-consistency.conf),
as well as passing in a feature key file to the cluster nodes.

```bash
$ ./aerolab cluster create -c 3 -o templates/strong-consistency.conf -f features.conf
```

## Apply the roster

```bash
$ ./aerolab roster apply -m bar
```
