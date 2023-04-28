[Docs home](../../README.md)


# Configure Shadow Devices


This example presents a scenario, in which:
- 2 NVME devices exist, to be partitioned into 4 partitions each.
- 8 EBS devices exist, to be used as shadows for the 8 total NVME partitions.

1. Create a cluster:

```
aerolab cluster create -n clusterName -c 2 -I r5ad.4xlarge -E 20,30,30,30,30,30,30,30,30
```

2. `blkdiscard` and partition NVME: 25% disk space on each partition.

```
aerolab cluster partition create -n clusterName --filter-type=nvme -p 25,25,25,25
```

3. prepare EBS by zeroing out the first 8MiB as required by Aerospike:

```
aerolab cluster partition create -n clusterName --filter-type=ebs
```

4. Configure Aerospike to use devices for `test` namespace

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=nvme --configure=device
```

5. Configure EBS to be used as shadow devices, no partitions:

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=ebs --filter-partitions=0 --configure=shadow
```

6. Restart Aerospike:

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```
