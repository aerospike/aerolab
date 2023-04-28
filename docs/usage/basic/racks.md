[Docs home](../../../README.md)

# Deploy a rack-aware cluster


AeroLab makes it easy to deploy a [rack-aware](/server/operations/configure/network/rack-aware)
Aerospike Database cluster.

### Create a 6-node Aerospike cluster, do not start `aerospike`

```bash
aerolab cluster create -c 6 -s n
```

### Assign rack-id 1 to first three nodes, all namespaces

```bash
aerolab conf rackid -l 1-3 -i 1 --no-restart
```

### Assign rack-id 2 to the next three nodes, all namespaces

```bash
aerolab conf rackid -l 4-6 -i 2 --no-restart
```

### Start `aerospike`

```bash
aerolab aerospike start
```

### Confirm the cluster is working

```bash
aerolab attach asadm -- -e info
```
