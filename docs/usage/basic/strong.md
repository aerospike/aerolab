[Docs home](../../../README.md)

# Strong consistency


## Create an Aerospike cluster with strong consistency

### Generate a template configuration file with strong consistency enabled

```bash
aerolab conf generate
```

Tick the box next to 'strong consistency' and hit CTRL+X to save `aerospike.conf`.

### Create a cluster, with a custom config and features file

```bash
$ ./aerolab cluster create -c 3 -o aerospike.conf -f features.conf
```

### Apply the roster

```bash
$ ./aerolab roster apply -m bar
```
