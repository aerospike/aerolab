[Docs home](../../../README.md)

# Node attach commands


AeroLab's `attach` commands give you shell access and direct tool access to the
Aerospike Database cluster nodes.

### Attach to the shell of cluster node 2

```bash
aerolab attach shell -n mycluster -l 2
```

### Attach to the shell of cluster node 2 and execute a command

```bash
aerolab attach shell -n mycluster -l 2 -- ls /
```

### Run AQL on cluster node 2

```bash
aerolab attach aql -n mycluster -l 2
```

### Run AQL on cluster node 2 with a command

```bash
aerolab attach aql -n mycluster -l 2 -- -c "show sets"
```

### Run asinfo on cluster node 1 with a command

```bash
aerolab attach asinfo -n mycluster -l 1 -- -v service
```

### Run asadm on cluster node 2

```bash
aerolab attach asadm -n mycluster -l 2
```

### Run asadm on cluster node 2 with a command

```bash
aerolab attach asadm -n mycluster -l 2 -- -e "info"
```

### Run AQL on all nodes

```bash
aerolab attach aql -n mycluster -l all -- -c "show sets"
```