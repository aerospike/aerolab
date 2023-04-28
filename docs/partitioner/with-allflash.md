[Docs home](../../README.md)


# Partition NVME and EBS for All-Flash Storage


This example presents a more complicated scenario, in which:
- 2 NVME devices exist, to be partitioned into 5 partitions each:
    - First partition of each NVME disk should be used for all-flash storage.
    - Remaining 4 partitions of each NVME disk should be used for device storage.
- 4 EBS devices exist, to be used as shadows for the 8 total NVME partitions.
  - Each EBS must consist of 2 partitions.

1. Create a cluster.

```
aerolab cluster create -n clusterName -c 2 -I r5ad.4xlarge -E 20,30,30,30,30
```

2. `blkdiscard` and partition NVME: 20% disk space on each partition.

```
aerolab cluster partition create -n clusterName --filter-type=nvme -p 20,20,20,20,20
```

3. `blkdiscard` and partition ebs: 50% disk space on each partition.

```
aerolab cluster partition create -n clusterName --filter-type=ebs -p 50,50
```

4. Configure Aerospike to use devices for `test` namespace, except first partition:

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=nvme --configure=device --filter-partitions=2-5
```

5. Configure EBS to be used as shadow devices, all partitions:

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=ebs --configure=shadow
```

6. Create a filesystem on partition 1 of each NVME disk, for all-flash:

```
aerolab cluster partition mkfs -n clusterName --filter-type=nvme --filter-partitions=1
```

7. Create or update all-flash Aerospike configuration on nodes:

```
aerolab cluster partition conf -n clusterName --filter-type=nvme --namespace=test --configure=allflash --filter-partitions=1
```

8. Restart Aerospike:

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```
