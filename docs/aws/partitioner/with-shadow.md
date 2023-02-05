# Partition NVME and EBS and configure shadow devices for a namespace

A more complicated example scenario, where:
* 2 nvme devices exist, to be partitioned into 4 partitions each
* 8 EBS devices exist, to be used as shadows for the 8 total nvme partitions

## create a cluster

```
aerolab cluster create -n clusterName -c 2 -I r5ad.4xlarge -E 20,30,30,30,30,30,30,30,30
```

## blkdiscard and partition nvme - 25% disk space on each partition

```
aerolab cluster partition create -n clusterName --filter-type=nvme -p 25,25,25,25
```

## prepare ebs (zero out the first 8MiB as required by aerospike)

```
aerolab cluster partition create -n clusterName --filter-type=ebs
```

## configure aerospike to use devices for 'test' namespace

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=nvme --configure=device
```

## configure EBS to be used as shadow devices, no partitions

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=ebs --filter-partitions=0 --configure=shadow
```

## restart aerospike

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```
