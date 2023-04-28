[Docs home](../../README.md)

# Use all disks for a namespace


1. Create a cluster:

```
aerolab cluster create -n clusterName -c 2 -I r5ad.large -E 20,30,30
```

2. `blkdiscard` and prepare the cluster:

```
aerolab cluster partition create -n clusterName
```

3. Configure Aerospike to use devices for `test` namespace:

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-partitions=0 --configure=device
```

4. Restart Aerospike:

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```