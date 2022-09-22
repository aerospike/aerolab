# Get logs from cluster

## Docker

```bash
aerolab logs get -n mydc
ls logs/
```

## AWS default

```bash
aerolab logs get -n mydc --journal
ls logs/
```

## Custom file location

```bash
aerolab logs get -n mydc -p /var/log/aerospike/aerospike.log
ls logs/
```

## Show logs instead of downloading

Same command line switches as above, replace `get` with `show`.

Add command line switch `-f` or `--follow` to follow logs.