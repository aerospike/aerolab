[Docs home](../../../README.md)

# TLS setup


AeroLab can assist you in [configuring TLS](/server/operations/configure/network/tls)
between client and server, as well as between server nodes.

### Create a two node cluster with a TLS configuration skeleton in place

Note: you can download the template configuration file from this repository, in the templates directory.

```bash
aerolab cluster create -o templates/tls.conf -c 2 -n mytest
```

### Generate TLS certificates

```bash
aerolab tls generate -n mytest
```

### Restart Aerospike

```bash
aerolab aerospike restart -n mytest
```

### Connect using AQL

Note: if using Docker Desktop, first run `aerolab cluster list` and grab the `ExposedPort` for node 1. Use that instead of `4333`.

```bash
aerolab attach shell -n mytest

# mutual auth off
aql --tls-enable --tls-cafile=/etc/aerospike/ssl/tls1/cacert.pem -h 127.0.0.1:tls1:4333

# mutual auth on
aql --tls-enable --tls-cafile=/etc/aerospike/ssl/tls1/cacert.pem --tls-keyfile=/etc/aerospike/ssl/tls1/key.pem --tls-certfile=/etc/aerospike/ssl/tls1/cert.pem -h 127.0.0.1:tls1:4333
```

### Notes on multiple certificates

`tls generate` will put certificates in the following path in the containers:

```
/etc/aerospike/{TLS_NAME}/cert.pem
/etc/aerospike/{TLS_NAME}/cacert.pem
/etc/aerospike/{TLS_NAME}/key.pem
```

As such, you can create and use multiple TLS names in your Aerospike config. For example:

```
network {
    tls tls1 {
    cert-file /etc/aerospike/ssl/tls1/cert.pem
    key-file /etc/aerospike/ssl/tls1/key.pem
    ca-file /etc/aerospike/ssl/tls1/cacert.pem
    }
    tls bob.domain.why.not {
    cert-file /etc/aerospike/ssl/bob.domain.why.not/cert.pem
    key-file /etc/aerospike/ssl/bob.domain.why.not/key.pem
    ca-file /etc/aerospike/ssl/bob.domain.why.not/cacert.pem
    }
```

If you use that in your template configuration file, snipped to make-cluster with the -o parameter, simply generate separate certificates for those 2 TLS names as follows:
```bash
aerolab tls generate -t tls1
aerolab tls generate -t bob.domain.why.not
```

### Other features

TLS generation allows for multiple CA certificates. If a CA cert already exists with the given name, it will be reused. If it doesn't, a new CA with that name will be generated.

AeroLab also has `tls copy` as a handy way to copy TLS certificates from one node to another (or one cluster to another).
