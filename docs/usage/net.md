# Network Control Commands
AeroLab's `net` commands allow you to simulate problems in inter-node and
inter-cluster networking.

## Block a port

Block node 1 in cluster `dc1` from talking on port `3000` to node 2 of cluster `dc2`; simulate packet loss through `drop` instead of `reject`.

```bash
aerolab net block -s dc1 -l 1 -d dc2 -i 2 -t drop -p 3000
```

## List network blocks

```bash
aerolab net list
```

## Unblock the previous network block

```bash
aerolab net unblock -s dc1 -l 1 -d dc2 -i 2 -t drop -p 3000
```

## Redo the network block, then trigger random packet loss

Same block as before, this time randomly dropping 3% of all packets

```bash
aerolab net block -s dc1 -l 1 -d dc2 -i 2 -t drop -p 3000 -M random -P 0.03
```

## Remove the network block

```bash
aerolab net unblock -s dc1 -l 1 -d dc2 -i 2 -t drop -p 3000 -M random -P 0.03
```

## Implement packet loss or packet latency

Switch | Meaning
--- | ---
s | Source cluster name
l | Source node list
d | Destination cluster name
i | Destination node list
a | action; set|del|delall|show
p | Latency
L | Packet loss count
R | Max link speed

```bash
aerolab net loss-delay -s dc1 -l 1 -d dc2 -i 2 -a set -p 100ms -L 10% -R 1024Kbps
```

## Show netowrk loss rules

```bash
aerolab net loss-delay -s dc1 -l 1 -d dc2 -l 2 -a show
```

## Delete all network loss rules between dc1 and dc2

```bash
aerolab net loss-delay -s dc1 -l 1,2,3 -d dc2 -i 1,2,3 -a del
```
