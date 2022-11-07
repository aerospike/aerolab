# Deploying the trino server

## Deploy trino

### Get cluster IPs

```
$ aerolab cluster list
```

### Deploy

Use `172.17.0.3:3000` as the Aerospike server seed IP:PORT

```
$ aerolab client create trino -n trino -s 172.17.0.3:3000
```

## Change which Aerospike cluster Trino communicates with

Use `172.17.0.4:3000` as seed IP:PORT

```
$ aerolab client configure trino -n trino -s 172.17.0.4:3000
```

## Attach to trino shell, namespace test

```
$ aerolab client attach trino -n trino -m test
```

## Attach to trino shell - the long way

```
$ aerolab client attach -n trino -- su - trino -c "bash ./trino --server 127.0.0.1:8080 --catalog aerospike --schema test"
```
