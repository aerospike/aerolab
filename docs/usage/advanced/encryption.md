[Docs home](../../../README.md)

# Encryption at rest


Each namespace in an Aerospike Database Enterprise Edition cluster can be configured with Encryption at Rest.

### Generate the configuration file

```
aerolab conf generate
```

Select the checkbox next to 'encryption at rest' and hit CTRL+X to save `aerospike.conf`.

### Create a cluster with the encryption at rest config template

Create a 3 node cluster with a custom configuration file, a custom feature license file and do not start aerospike yet.

```bash
$ aerolab cluster create -c 3 -o aerospike.conf -f /path/to/feature.conf -s n
```

### Generate an encryption key on your machine
```bash
# mac:
$ head -c 256 /dev/urandom >key.dat
# linux/WSL2:
$ head --bytes 256 /dev/urandom >key.dat
```

### Copy your encryption key to the cluster nodes
```bash
$ aerolab files upload key.dat /etc/aerospike/key.dat
```

### Start the Aerospike Database
```bash
$ aerolab aerospike start
```
