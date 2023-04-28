[Docs home](../../../README.md)

# Encryption at rest


Each namespace in an Aerospike Database Enterprise Edition cluster can be
[configured with Encryption at Rest](/server/operations/configure/security/encryption-at-rest).

### Create a cluster with the encryption at rest config template
In this example you will create a three node Aerospike cluster using the encryption at
rest [template](https://github.com/aerospike/aerolab/templates/encryption-at-rest.conf), and provide
a feature file as well.

```bash
$ aerolab cluster create -c 3 -o templates/encryption-at-rest.conf -f /path/to/feature.conf -s n
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
