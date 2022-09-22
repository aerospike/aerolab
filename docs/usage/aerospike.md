# Aerospike commands

## Start aerospike

```bash
aerolab aerospike start -n mycluster
```

## Stop aerospike

```bash
aerolab aerospike stop -n mycluster
```

## Restart aerospike

```bash
aerolab aerospike restart -n mycluster
```

## In-place upgrade aerospike on a single node 2

```bash
aerolab aerospike upgrade -n mycluster -l 2 -v 5.7.0.6 
```

## Restart aerospike on a single node 2

```bash
aerolab aerospike restart -n mycluster -l 2
```
