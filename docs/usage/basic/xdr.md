[Docs home](../../../README.md)

# Cross-datacenter replication


## Set up cross-datacenter replication between clusters
AeroLab makes it easy to deploy distinct clusters that are connected through
cross-datacenter replication ([XDR](/server/operations/configure/cross-datacenter)).

### Create two Aerospike clusters with XDR between them

Switch | Meaning
--- | ---
s | Source cluster name
c | Number of source nodes
x | Destination cluster name
a | Number of destination nodes
m | List of namespaces to connect
v | Server version to deploy (default: latest)

```bash
aerolab xdr create-clusters -n dc1 -c 3 -N dc2 -C 3 -M test,bar -v 5.7.0.12
```

### Destroy both clusters

```bash
aerolab cluster destroy -n dc1,dc2 -f
```

### Create Aerospike clusters manually and join them with XDR

```bash
aerolab cluster create -n dc1 -c 3 -v 4.9.0.32
aerolab cluster create -n dc2 -c 3 -v 4.9.0.32
aerolab xdr connect -S dc1 -D dc2 -M test,bar
```
