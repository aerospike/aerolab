# Make/Grow cluster with resource limits

## Note on ram-limit and swap-limit

* ram-limit is the amount of RAM memory the node can use
* swap-limit is the amount of RAM+SWAP memory the node can use
* as such, to set 300m RAM and 100m swap, ram-limit=300m, swap-limit=400m (300+100)

## Instructions

### Make a 2-node cluster, giving each node 0.5 of CPU and 500mb of ram + 100mb of swap access
```bash
$ ./aerolab cluster create -c 2 --cpu-limit=0.5 --ram-limit=500m --swap-limit=600m
```

### Grow cluster by one node with a 2-core limit, 1gb of RAM and no swap
```bash
$ ./aerolab cluster grow -c 1 --cpu-limit=2 --ram-limit=1g --swap-limit=1g
```

### Grow cluster by one node with a 0.1-cpu limit, 1gb of RAM and unlimited swap
```bash
$ ./aerolab cluster grow -c 1 --cpu-limit=0.1 --ram-limit=1g
```
