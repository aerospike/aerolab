# Deploy a Rack-Aware Cluster
AeroLab makes it easy for you to deploy a [rack-aware](https://docs.aerospike.com/server/operations/configure/network/rack-aware)
Aerospike Database cluster.

## Create a size node Aerospike cluster, do not start aerospike

```bash
aerolab cluster create -c 6 -s n
```

## Assign rack-id 1 to first three nodes, all namespaces

```bash
aerolab conf rackid -l 1-3 -i 1
```

## Assign rack-id 2 to the next three nodes, all namespaces

```bash
aerolab conf rackid -l 4-6 -i 2
```

## Start aerospike

```bash
aerolab aerospike start
```

## Confirm the cluster is working

```bash
aerolab attach asadm -- -e info
```
