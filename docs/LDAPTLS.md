## Make a TLS cluster with LDAP

### Deploy LDAP server on docker

Get and run [deploy-ldap.sh](/scripts/deploy-ldap.sh) file.
```bash
$ ./deploy-ldap.sh
```

### Change ldap.conf to point aerospike at the ldap server
```bash
$ LDAPIP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ldap_server)
$ sed "s/LDAPIP/$LDAPIP/g" templates/ldap_tls.conf > templates/ldaptls_custom.conf
```

### Deploy aerospike with 2 nodes and ldap template

```bash
$ aerolab make-cluster -n ldap -c 2 -o templates/ldaptls_custom.conf -f features.conf -m mesh
$ rm templates/ldaptls_custom.conf
```

### Generate TLS certificates
```
$ ./aerolab gen-tls-certs -n ldap
```

### Restart aerospike
```
$ ./aerolab restart-aerospike -n ldap
```

### Test that the connection is working
```bash
$ aerolab node-attach -n ldap -- aql --auth EXTERNAL_INSECURE -Ubadwan -Pblastoff --tls-enable --tls-cafile=/etc/aerospike/ssl/tls1/cacert.pem -h 127.0.0.1:tls1:4333 -c "show bins"
```

### Remove cluster

```bash
$ aerolab cluster-destroy -f 1 -n ldap
```

### Remove and kill the ldap server

```bash
$ docker stop ldap_server ; docker rm ldap_server
```
