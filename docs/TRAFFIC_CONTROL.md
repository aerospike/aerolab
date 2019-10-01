# Traffic control

## Start new clusters

```bash
$ aerolab make-xdr-clusters -c 3 -a 3
May 21 11:48:34+0000 AERO-LAB[2185]: INFO     --> Deploying dc1
May 21 11:48:34+0000 AERO-LAB[2185]: INFO     Performing sanity checks, checking if docker/lxc is running and accessible
May 21 11:48:34+0000 AERO-LAB[2185]: INFO     Checking if version template already exists
May 21 11:48:34+0000 AERO-LAB[2185]: INFO     Checking aerospike version
May 21 11:48:43+0000 AERO-LAB[2185]: INFO     Starting deployment
May 21 11:48:51+0000 AERO-LAB[2185]: INFO     Done
May 21 11:48:51+0000 AERO-LAB[2185]: INFO     --> Deploying dc2
May 21 11:48:51+0000 AERO-LAB[2185]: INFO     Performing sanity checks, checking if docker/lxc is running and accessible
May 21 11:48:51+0000 AERO-LAB[2185]: INFO     Checking if version template already exists
May 21 11:48:51+0000 AERO-LAB[2185]: INFO     Checking aerospike version
May 21 11:49:03+0000 AERO-LAB[2185]: INFO     Starting deployment
May 21 11:49:11+0000 AERO-LAB[2185]: INFO     Done
May 21 11:49:11+0000 AERO-LAB[2185]: INFO     --> xdrConnect running
May 21 11:49:11+0000 AERO-LAB[2185]: INFO     XdrConnect running
May 21 11:49:14+0000 AERO-LAB[2185]: INFO     Done, now restart the source cluster for changes to take effect.
$ aerolab restart-aerospike -n dc1

```

## Make dc1 nodes 1,2 max rate 100Kbps to dc2 nodes 2,3

```bash
$ aerolab net-loss-delay -s dc1 -l 1,2 -d dc2 -i 2,3 -a set --rate=100Kbps
May 21 11:50:34+0000 AERO-LAB[2304]: INFO     Starting net loss/delay
May 21 11:50:38+0000 AERO-LAB[2304]: INFO     Done
```

## Make dc1 node 2 delay (latency) to dc2 node 1

```bash
$ aerolab net-loss-delay -s dc1 -l 2 -d dc2 -i 1 -a set --delay=150ms
May 21 11:50:34+0000 AERO-LAB[2304]: INFO     Starting net loss/delay
May 21 11:50:38+0000 AERO-LAB[2304]: INFO     Done
```

## Make dc1 node 3 loose 20% of all packets to dc2 node 1

```bash
$ aerolab net-loss-delay -s dc1 -l 3 -d dc2 -i 1 -a set --loss=20%
May 21 11:50:34+0000 AERO-LAB[2304]: INFO     Starting net loss/delay
May 21 11:50:38+0000 AERO-LAB[2304]: INFO     Done
```

## Show all the rules on dc1

```bash
$ aerolab net-loss-delay -s dc1 -l 1,2,3 -a show --show-names
May 21 11:53:28+0000 AERO-LAB[2363]: INFO     Starting net loss/delay
========== cluster dc1 node 1 ==========
{
    "eth0": {
        "outgoing": {
            "dst-network=CLUSTER=dc2 NODE=2, protocol=ip": {
                "filter_id": "800::800",
                "rate": "100Kbps"
            },
            "dst-network=CLUSTER=dc2 NODE=3, protocol=ip": {
                "filter_id": "800::801",
                "rate": "100Kbps"
            }
        },
        "incoming": {}
    }
}

========== cluster dc1 node 2 ==========
{
    "eth0": {
        "outgoing": {
            "dst-network=CLUSTER=dc2 NODE=2, protocol=ip": {
                "filter_id": "800::800",
                "rate": "100Kbps"
            },
            "dst-network=CLUSTER=dc2 NODE=3, protocol=ip": {
                "filter_id": "800::801",
                "rate": "100Kbps"
            },
            "dst-network=CLUSTER=dc2 NODE=1, protocol=ip": {
                "filter_id": "800::802",
                "delay": "150.0ms",
                "rate": "10Gbps"
            }
        },
        "incoming": {}
    }
}

========== cluster dc1 node 3 ==========
{
    "eth0": {
        "outgoing": {
            "dst-network=CLUSTER=dc2 NODE=1, protocol=ip": {
                "filter_id": "800::800",
                "loss": "20%",
                "rate": "10Gbps"
            }
        },
        "incoming": {}
    }
}

May 21 11:54:30+0000 AERO-LAB[2404]: INFO     Done
```

## Clear all rules on dc1 node 1

NOTE: this error can be safely ignored, it's just an INFO/NOTICE

```bash
$ aerolab net-loss-delay -s dc1 -l 1 -a delall
May 21 11:56:22+0000 AERO-LAB[2552]: INFO     Starting net loss/delay
May 21 11:56:23+0000 AERO-LAB[2552]: ERROR    cluster dc1 node 1 ERROR running [exec aero-dc1_1 /bin/bash -c source /tcconfig/bin/activate; tcdel eth0 --all]: exit status 1
 [INFO] tcconfig: delete eth0 qdisc
[NOTICE] tcconfig: no qdisc to delete for the incoming device.

May 21 11:56:23+0000 AERO-LAB[2552]: INFO     Done
```

## Remove rules from dc1 node 2 to dc2 nodes 1,3

```bash
$ aerolab net-loss-delay -s dc1 -l 2 -d dc2 -i 1,3 -a del
May 21 11:58:11+0000 AERO-LAB[2592]: INFO     Starting net loss/delay
May 21 11:58:13+0000 AERO-LAB[2592]: INFO     Done
```

## Show rules again

```bash
$ aerolab net-loss-delay -s dc1 -l 1,2,3 -a show --show-names
May 21 11:59:02+0000 AERO-LAB[2611]: INFO     Starting net loss/delay
========== cluster dc1 node 1 ==========
{
    "eth0": {
        "outgoing": {},
        "incoming": {}
    }
}

========== cluster dc1 node 2 ==========
{
    "eth0": {
        "outgoing": {
            "dst-network=CLUSTER=dc2 NODE=2, protocol=ip": {
                "filter_id": "800::800",
                "rate": "100Kbps"
            }
        },
        "incoming": {}
    }
}

========== cluster dc1 node 3 ==========
{
    "eth0": {
        "outgoing": {
            "dst-network=CLUSTER=dc2 NODE=1, protocol=ip": {
                "filter_id": "800::800",
                "loss": "20%",
                "rate": "10Gbps"
            }
        },
        "incoming": {}
    }
}

May 21 11:59:05+0000 AERO-LAB[2611]: INFO     Done
```

## Destroy clusters

```bash
aerolab cluster-destroy -f -n dc1,dc2
```
