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
# aws
aerolab cluster create -n clusterName -c 2 -I r5ad.4xlarge -E 20,30,30,30,30

# gcp
aerolab cluster create -n clusterName -c 2 --instance c2d-standard-16 --zone us-central1-a --disk=pd-ssd:20 --disk=local-ssd --disk=local-ssd --disk=pd-ssd:30 --disk=pd-ssd:30 --disk=pd-ssd:30 --disk=pd-ssd:30
```

2. `blkdiscard` and partition NVME: 16% disk space on each partition.

```
aerolab cluster partition create -n clusterName --filter-type=nvme -p 16,16,16,16,16,16
```

3. `blkdiscard` and partition ebs: 50% disk space on each partition.

```
aerolab cluster partition create -n clusterName --filter-type=ebs -p 50,50
```

4. Configure Aerospike to use devices for `test` namespace, except first 2 partitions:

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=nvme --configure=device --filter-partitions=3-6
```

5. Configure EBS to be used as shadow devices, all partitions:

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=ebs --configure=shadow
```

6. Create a filesystem on partitions 1,2 of each NVME disk, for allflash:

```
aerolab cluster partition mkfs -n clusterName --filter-type=nvme --filter-partitions=1,2
```

7. Create or update all-flash Aerospike configuration on nodes (partition1=pi-flash and partition2=si-flash):

```
aerolab cluster partition conf -n clusterName --filter-type=nvme --namespace=test --configure=pi-flash --filter-partitions=1
aerolab cluster partition conf -n clusterName --filter-type=nvme --namespace=test --configure=si-flash --filter-partitions=2
```

8. Restart Aerospike:

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```
