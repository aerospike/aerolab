# Network control commands

## Block a port

Block `dc1` node 1 from talking on port `3000` to `dc2` node 2, simulate package loss thrrough drop isntead of reject.

```bash
aerolab net-block -s dc1 -l 1 -d dc2 -i 2 -t drop -p 3000
```

## List blocks

```bash
aerolab net-list
```

## Unblock the previous block

```bash
aerolab net-unblock -s dc1 -l 1 -d dc2 -i 2 -t drop -p 3000
```

## Redo the block, random loss

Same block as before, this time randomly dropping 3% of all packets

```bash
aerolab net-block -s dc1 -l 1 -d dc2 -i 2 -t drop -p 3000 -M random -P 0.03
```

## Remove the block

```bash
aerolab net-unblock -s dc1 -l 1 -d dc2 -i 2 -t drop -p 3000 -M random -P 0.03
```

## Implement network loss or packet latencies

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
aerolab net-loss-delay -s dc1 -l 1 -d dc2 -i 2 -a set -p 100ms -L 10% -R 1024Kbps
```

## Show netowrk loss rules

```bash
aerolab net-loss-delay -s dc1 -l 1 -d dc2 -l 2 -a show
```

## Delete all rules between dc1 and dc2

```bash
aerolab net-loss-delay -s dc1 -l 1,2,3 -d dc2 -i 1,2,3 -a del
```
