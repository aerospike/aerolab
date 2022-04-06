# Insert and Delete data

While `asbench` can be used to benchmark clusters and perform specific workloads, `aerolab` provides a simple way to insert and delete data with a more lab-test approach.

## Insert data

Insert 100'000 records into namespace `test` set name `myset`

```bash
aerolab insert-data -m test -s myset -a 1 -z 100000
```

## Durable Delete data

Delete only the first 50'000 records

```bash
aerolab delete-data -m test -s myset -a 1 -z 50000 -D
```

## Note on features

For specific needs, explore the `aerolab insert-data help` and `aerolab delete-data help`

The `insert-data` function has a large number of features, including selection of bin names and values to insert data, read-after-write option, TLS and auth support, TTL support and an option to force data load only to a specific set of nodes or partitions.
