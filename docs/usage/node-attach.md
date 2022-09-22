# Node Attach commands

## Attach to node 2 shell

```bash
aerolab attach shell -n mycluster -l 2
```

## Attach to node 2 shell and execute a command

```bash
aerolab attach shell -n mycluster -l 2 -- ls /
```

## Run aql on node 2

```bash
aerolab attach aql -n mycluster -l 2
```

## Run aql on node 2 with a command

```bash
aerolab attach aql -n mycluster -l 2 -- -c "show sets"
```

## Run asinfo on node 1 with a command

```bash
aerolab attach asinfo -n mycluster -l 1 -- -v service
```

## Run asadm on node 2

```bash
aerolab attach asadm -n mycluster -l 2
```

## Run asadm on node 2 with a command

```bash
aerolab attach asadm -n mycluster -l 2 -- -e "info"
```
