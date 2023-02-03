# Partition each NVME into 4 partitions and use 2 for each of the two namespaces

## blkdiscard and partition - 25% disk space on each partition

```
aerolab cluster partition create -n clusterName --filter-type=nvme -p 25,25,25,25
```

## configure aerospike to use devices for 'test' and 'bar' namespaces

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=nvme --filter-partitions=1,2 --configure=device
aerolab cluster partition conf -n clusterName --namespace=bar --filter-type=nvme --filter-partitions=3,4 --configure=device
```

## restart aerospike

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```
