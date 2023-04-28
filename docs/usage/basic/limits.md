[Docs home](../../../README.md)

# Limit cluster resource use


### Note on RAM limit and swap limit

* `ram-limit` is the amount of RAM memory the node can use
* `swap-limit` is the amount of RAM+SWAP memory the node can use
* as such, to set 300MiB RAM and 100MiB swap, `ram-limit=300m`, `swap-limit=400m` (300MiB + 100MiB)

### Instructions

#### Create a two node cluster, giving each node 0.5 of a CPU and 500MiB of RAM + 100MiB of swap access
```bash
$ ./aerolab cluster create -c 2 --cpu-limit=0.5 --ram-limit=500m --swap-limit=600m
```

#### Grow the cluster by one node with a two core limit, 1GiB of RAM and no swap
```bash
$ ./aerolab cluster grow -c 1 --cpu-limit=2 --ram-limit=1g --swap-limit=1g
```

#### Grow the cluster by one node with a 0.1 CPU limit, 1GiB of RAM and unlimited swap
```bash
$ ./aerolab cluster grow -c 1 --cpu-limit=0.1 --ram-limit=1g
```
