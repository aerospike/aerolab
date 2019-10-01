# Make 2-node cluster with TLS

### Make the cluster with tls configuration in place 
```
$ ./aerolab make-cluster -o templates/tls.conf -c 2
Nov 06 09:25:12+0000 AERO-LAB[13734]: INFO     Performing sanity checks
Nov 06 09:25:13+0000 AERO-LAB[13734]: INFO     Checking if version template already exists
Nov 06 09:25:13+0000 AERO-LAB[13734]: INFO     Checking aerospike version
Nov 06 09:25:25+0000 AERO-LAB[13734]: INFO     Starting deployment
Nov 06 09:25:30+0000 AERO-LAB[13734]: INFO     Done
```

### Generate TLS certificates
```
$ ./aerolab gen-tls-certs
Nov 06 09:25:59+0000 AERO-LAB[13764]: INFO     Generating TLS certificates and reconfiguring hosts
Nov 06 09:26:04+0000 AERO-LAB[13764]: INFO     Done
```

### Restart aerospike
```
$ ./aerolab restart-aerospike
```

### Notes on multiple certificates

gen-tls-certs will put certificates in the following path in the containers:
```
/etc/aerospike/{TLS_NAME}/cert.pem
/etc/aerospike/{TLS_NAME}/cacert.pem
/etc/aerospike/{TLS_NAME}/key.pem
```

As such, you can create and use multiple TLS names in your aerospike config. For example:
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

if you use that in your template conf file, snipped to make-cluster with the -o parameter, simply generate separate certificates for those 2 TLS names as follows:
```bash
$ ./aerolab gen-tls-certs -t tls1
$ ./aerolab gen-tls-certs -t bob.domain.why.not
```
