# How to enable encryption at rest

## Create a cluster using the templates, say 3 nodes, providing feature file too
```bash
$ aerolab cluster create -c 3 -o templates/encryption-at-rest.conf -f feature.conf -s n
```

## Generate encryption key on your machine
```bash
# mac:
$ head -c 256 /dev/urandom >key.dat
# linux/WSL2:
$ head --bytes 256 /dev/urandom >key.dat
```

## Copy key to the nodes in the cluster:
```bash
$ aerolab files upload key.dat /etc/aerospike/key.dat
```

## Start aerospike
```bash
$ ./aerolab aerospike start
```
