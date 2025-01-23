[Docs home](../../../README.md)

# Strong consistency

Create an Aerospike cluster with strong consistency

## Create a cluster

```
# docker
aerolab config backend -t docker
aerolab cluster create -f features.conf

# aws
aerolab config backend -t aws -r us-west-2
aerolab cluster create -I t3a.large -f features.conf

# gcp
aerolab config backend -t gcp -o aerolab-test-project-1
aerolab cluster create --instance e2-standard-2 --zone us-central1-a -f features.conf
```

## Apply strong consistency

```
aerolab conf sc
```

## Further options

The `aerolab conf sc` command allows for custom namespace names, addition of rack-aware configuration as well as auto-partitioning of devices for a namespace (should the instance type have devices).

```
$ aerolab conf sc help
    [...]
    -n, --name=       Cluster name (default: mydc)
    -m, --namespace=  Namespace to change (default: test)
    -p, --path=       Path to aerospike.conf (default: /etc/aerospike/aerospike.conf)
    -f, --force       If set, will zero out the devices even if strong-consistency was already configured
    -r, --racks=      If rack-aware feature is required, set this to the number of racks you want to divide the cluster into
    -d, --with-disks  If set, will attempt to configure device storage engine for the namespace, using all available devices
    -t, --threads=    Run on this many nodes in parallel (default: 50)
```

Full example:

```bash
# create the cluster, 9 nodes
aerolab config backend -t aws -r us-west-2
aerolab cluster create -I m5d.large -f features.conf -c 9

# configure SC with 'test' namespace, 3 racks (which results in 3 nodes / rack), and using all available devices
aerolab conf sc -m test -r 3 -d
```
