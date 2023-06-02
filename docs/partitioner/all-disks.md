[Docs home](../../README.md)

# Use all disks for a namespace


1. Create a cluster:

```
# aws
aerolab cluster create -n clusterName -c 2 -I r5ad.large -E 20,30,30

# gcp - root volume: 50GB, 2x local SSD, 2x persistent SSD 380GB
aerolab cluster create -n clusterName -c 2 --instance c2d-standard-16 --zone us-central1-a --disk=pd-ssd:50 --disk=local-ssd --disk=local-ssd --disk=pd-ssd:380 --disk=pd-ssd:380
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