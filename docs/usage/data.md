# Insert and Delete Data

While `asbench` can be used to benchmark Aerospike clusters and perform specific workloads, AeroLab provides a simple way to insert and delete data with a more lab-test approach.

## Insert data

Insert 100K records into namespace `test`, set `myset`

```bash
aerolab data insert -m test -s myset -a 1 -z 100000
```

## Durable-delete data

Delete only the first 50K records

```bash
aerolab data delete -m test -s myset -a 1 -z 50000 -D
```

## Note on features

For specific needs, explore `aerolab data insert help` and `aerolab data delete help`

The `data insert` function has a large number of features, including selection of bin names and values to insert data, read-after-write option, TLS and auth support, TTL support and an option to force data load only to a specific set of nodes or partitions.
