# Make cluster with strong consistency

## Create cluster, shipping custom config and features file

```bash
$ ./aerolab cluster create -c 3 -o templates/strong-consistency.conf -f features.conf
```

## Apply roster

```bash
$ ./aerolab roster apply
```
