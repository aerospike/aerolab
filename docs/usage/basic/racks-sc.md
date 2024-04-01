[Docs home](../../../README.md)

# Deploy a rack-aware cluster with strong consistency


AeroLab simplifies deploying a [rack-aware](https://aerospike.com/docs/server/reference/configuration?search=rack-id&context=namespace&version=all#namespace__rack-id)
Aerospike Database cluster in [strong consistency mode](https://aerospike.com/docs/server/reference/configuration?search=strong-consistency&context=namespace&version=all).

### Create a custom template configuration file

```
aerolab conf generate -f sc-template.conf
```

Select strong consistency tickbox, and hit CTRL+X to save the file.

### Create a 6-node Aerospike cluster, do not start `aerospike`

```bash
aerolab cluster create -c 6 -s n -o sc-template.conf
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
