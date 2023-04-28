[Docs home](../../../README.md)

# Aerospike database commands

AeroLab's `aerospike` commands deal with starting and stopping the Aerospike
server daemon ([asd](https://docs.aerospike.com/server/operations/manage/aerospike))
on the Aerospike Database cluster nodes.

### Start an Aerospike cluster

```bash
aerolab aerospike start -n mycluster
```

### Stop an Aerospike cluster

```bash
aerolab aerospike stop -n mycluster
```

### Restart an Aerospike cluster

```bash
aerolab aerospike restart -n mycluster
```

### Upgrade node 2 of the Aerospike cluster in-place

```bash
aerolab aerospike upgrade -n mycluster -l 2 -v 5.7.0.6
```

### Restart node 2 of the Aerospike cluster

```bash
aerolab aerospike restart -n mycluster -l 2
```
