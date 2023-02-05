# Partition NVME and EBS, use all partitions for namespace, with first nvme partition of each nvme disk for all-flash

A more complicated example scenario, where:
* 2 nvme devices exist, to be partitioned into 5 partitions each
    * first partition of each NVME to be used for all-flash
    * remaining 4 partitions of each NVME to be used for device storage
* 4 EBS devices exist, to be used as shadows for the 8 total nvme partitions
  * which means each EBS must consist of 2 partitions

## create a cluster

```
aerolab cluster create -n clusterName -c 2 -I r5ad.4xlarge -E 20,30,30,30,30
```

## blkdiscard and partition nvme - 20% disk space on each partition

```
aerolab cluster partition create -n clusterName --filter-type=nvme -p 20,20,20,20,20
```

## blkdiscard and partition ebs - 50% disk space on each partition

```
aerolab cluster partition create -n clusterName --filter-type=ebs -p 50,50
```

## configure aerospike to use devices for 'test' namespace, except first partition

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=nvme --configure=device --filter-partitions=2-5
```

## configure EBS to be used as shadow devices, all partitions

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=ebs --configure=shadow
```

## create a filesystem on partition 1 of each nvme, for all-flash

```
aerolab cluster partition mkfs -n clusterName --filter-type=nvme --filter-partitions=1
```

## create or update all-flash aerospike configuration on nodes

```
aerolab cluster partition conf -n clusterName --filter-type=nvme --namespace=test --configure=allflash --filter-partitions=1
```

## restart aerospike

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```
