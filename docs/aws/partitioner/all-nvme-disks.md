# Partitioner - use all nvme disks for a namespace

## blkdiscard and prepare

```
aerolab cluster partition create -n clusterName --filter-type=nvme
```

## configure aerospike to use devices for 'test' namespace

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=nvme --filter-partitions=0 --configure=device
```

## restart aerospike

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```
