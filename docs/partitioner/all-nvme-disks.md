[Docs home](../../README.md)


# Use all NVME disks for a namespace


1 Create a cluster:

```
# aws
aerolab cluster create -n clusterName -c 2 -I r5ad.4xlarge

# gcp - root volume: 50GB, 2x local SSD
aerolab cluster create -n clusterName -c 2 --instance c2d-standard-16 --zone us-central1-a --disk=pd-ssd:50 --disk=local-ssd --disk=local-ssd
```

2. `blkdiscard` and prepare the cluster:

```
aerolab cluster partition create -n clusterName --filter-type=nvme
```

3. Configure Aerospike to use devices for `test` namespace:

```
aerolab cluster partition conf -n clusterName --namespace=test --filter-type=nvme --filter-partitions=0 --configure=device
```

4. Restart Aerospike:

```
aerolab aerospike stop -n clusterName
aerolab aerospike start -n clusterName
```
