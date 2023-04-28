[Docs home](../../../README.md)

# Logging


Read more about [Configuring Log Files](/server/operations/configure/logging/configure_log_files.md)

### Logs from a Docker container

```bash
aerolab logs get -n mydc
ls logs/
```

### Logs from journald on AWS

```bash
aerolab logs get -n mydc --journal
ls logs/
```

### Logs from a custom log file location

```bash
aerolab logs get -n mydc -p /var/log/aerospike/aerospike.log
ls logs/
```

### Showing logs instead of downloading

To show rather than fetch the logs, replace `get` with `show` in the examples
above.

Add the command line switch `-f` or `--follow` to tail the logs.
