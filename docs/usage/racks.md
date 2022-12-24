# Deploy a Rack-Aware Cluster
AeroLab makes it easy for you to deploy a [rack-aware](https://docs.aerospike.com/server/operations/configure/network/rack-aware)
Aerospike Database cluster.

## Create a three node Aerospike cluster with rack-id 1

```bash
aerolab cluster create -c 3 -o templates/rack1.conf
```

## Grow the cluster by another three nodes, using rack-id 2

```bash
aerolab cluster grow -c 3 -o templates/rack2.conf
```

## Confirm the cluster is working

```bash
aerolab attach asadm -- -e info
```
