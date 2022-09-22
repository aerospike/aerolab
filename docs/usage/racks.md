# Deploy a rack-aware cluster

## Create a 3-node cluster with rack-id 1

```bash
aerolab cluster create -c 3 -o templates/rack1.conf
```

## Grow the cluster by another 3 nodes, using rack-id 2

```bash
aerolab cluster grow -c 3 -o templates/rack2.conf
```

## Confirm cluster is working

```bash
aerolab attach asadm -- -e info
```
