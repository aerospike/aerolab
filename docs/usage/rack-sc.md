# Deploy a Rack-Aware Cluster with strong-consistency
AeroLab makes it easy for you to deploy a [rack-aware](https://docs.aerospike.com/server/operations/configure/network/rack-aware)
Aerospike Database cluster.

## Create a size node Aerospike cluster, do not start aerospike

```bash
aerolab cluster create -c 6 -s n -o sc-template-file.conf
```

## Assign rack-id 1 to first three nodes, all namespaces

Do not re-roster with strong-consistency, we are not ready to do that yet.

```bash
aerolab conf rackid -l 1-3 -i 1 --no-roster
```

## Assign rack-id 2 to the next three nodes, all namespaces

This time we will allow strong consistency roster to be apply too and the cluster to be restarted.

```bash
aerolab conf rackid -l 4-6 -i 2
```

## Confirm the cluster is working

```bash
aerolab attach asadm -- -e info
```
