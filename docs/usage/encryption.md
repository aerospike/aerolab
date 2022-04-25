# How to enable encryption at rest

## Create a cluster using the templates, say 3 nodes, providing feature file too
```bash
$ ./aerolab make-cluster -c 3 -o templates/encryption-at-rest.conf -f feature.conf -s n
Nov 20 15:01:25+0000 AERO-LAB[60062]: INFO     Performing sanity checks, checking if docker is running and accessible
Nov 20 15:01:25+0000 AERO-LAB[60062]: INFO     Checking if version template already exists
Nov 20 15:01:25+0000 AERO-LAB[60062]: INFO     Checking aerospike version
Nov 20 15:01:32+0000 AERO-LAB[60062]: INFO     Starting deployment
Nov 20 15:01:40+0000 AERO-LAB[60062]: INFO     Done
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
$ ./aerolab upload -i key.dat -o /etc/aerospike/key.dat
```

## Start aerospike
```bash
$ ./aerolab start-aerospike
```
