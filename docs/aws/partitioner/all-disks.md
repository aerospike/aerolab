# Partitioner - use all disks for a namespace

## create a cluster

```
aerolab cluster create -n clusterName -c 2 -I r5ad.large -E 20,30,30
```

## blkdiscard and prepare

```
aerolab cluster partition create -n clusterName
```

## configure aerospike to use devices for 'test' namespace

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-partitions=0 --configure=device
```

## restart aerospike

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```
