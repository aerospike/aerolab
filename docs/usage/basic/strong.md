[Docs home](../../../README.md)

# Strong consistency

## Prerequisites

* the node count must be equal or higher to the configured `replication-factor`
* the feature file `features.conf` must support the number of nodes being deployed in the cluster, or the cluster won't form

For simple testing, ensure the replication factor is `1` in the custom `aerospike.conf`.

## Create an Aerospike cluster with strong consistency

### Generate a template configuration file with strong consistency enabled

```bash
aerolab conf generate
```

Tick the box next to 'strong consistency' and hit CTRL+X to save `aerospike.conf`.

### Replication factor and test features file limitations

Optionally edit `aerospike.conf` and set `replication-factor 1` for the namespace, in order to allow 1-node cluster with SC.

### Create a cluster, with a custom config and features file

```bash
$ ./aerolab cluster create -c 3 -o aerospike.conf -f features.conf
```

### Apply the roster

```bash
$ ./aerolab roster apply -m test
```
