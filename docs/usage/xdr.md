# Setup XDR

## Create 2 clusters with XDR between them

Switch | Meaning
--- | ---
s | Source cluster name
c | Number of source nodes
x | Destination cluster name
a | Number of destination nodes
m | List of namespaces to connect
v | Server version to deploy (default: latest)

```bash
aerolab make-xdr-clusters -s dc1 -c 3 -x dc2 -a 3 -m test,bar -v 5.7.0.12
aerolab restart-aerospike -n dc1
```

## Destroy both clusters

```bash
aerolab cluster-destroy -n dc1,dc2 -f
```

## Create clusters manually and join xdr

Note, if connecting clusters with version 5+, `xdr-connect` requires the `-5` switch as well.

```bash
aerolab make-cluster -n dc1 -c 3 -v 4.9.0.32
aerolab make-cluster -n dc2 -c 3 -v 4.9.0.32
aerolab xdr-connect -s dc1 -d dc2 -m test,bar
aerolab restart-aerospike -n dc1
```
