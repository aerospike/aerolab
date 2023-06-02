[Docs home](../../README.md)


# Create Partitioning for Two Namespaces


1. Create a cluster:

```
#aws
aerolab cluster create -n clusterName -c 2 -I r5ad.4xlarge

# gcp - root volume: 50GB, 2x local SSD
aerolab cluster create -n clusterName -c 2 --instance c2d-standard-16 --zone us-central1-a --disk=pd-ssd:50 --disk=local-ssd --disk=local-ssd
```

2. `blkdiscard` and partition, 25% disk space on each partition:

```
aerolab cluster partition create -n clusterName --filter-type=nvme -p 25,25,25,25
```

3. Configure Aerospike to use devices for `test` and `bar` namespaces:

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=nvme --filter-partitions=1,2 --configure=device
aerolab cluster partition conf -n clusterName --namespace=bar --filter-type=nvme --filter-partitions=3,4 --configure=device
```

4. Restart Aerospike:

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```
