[Docs home](../../../README.md)

# Deploy a rack-aware cluster with strong consistency


AeroLab simplifies deploying a [rack-aware](/server/operations/configure/network/rack-aware)
Aerospike Database cluster in [strong consistency mode](/server/architecture/consistency).

### Create a 6-node Aerospike cluster, do not start `aerospike`

```bash
aerolab cluster create -c 6 -s n -o SC-TEMPLATE-FILE.CONF
```

## Assign rack-id 1 to first three nodes, all namespaces

Do not re-roster with strong consistency yet.

```bash
aerolab conf rackid -l 1-3 -i 1 --no-roster
```

## Assign rack-id 2 to the next three nodes, all namespaces

This time, apply the strong consistency roster and start the cluster.

```bash
aerolab conf rackid -l 4-6 -i 2
```

## Confirm the cluster is working

```bash
aerolab attach asadm -- -e info
```