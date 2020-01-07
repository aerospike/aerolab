## Deploying LDAP on docker

### Deploy LDAP server on docker

Get and run [deploy-ldap.sh](/scripts/deploy-ldap.sh) file.
```bash
$ ./deploy-ldap.sh
```

### Change ldap.conf to point aerospike at the ldap server
```bash
$ LDAPIP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ldap_server)
$ sed "s/LDAPIP/$LDAPIP/g" templates/ldap.conf > templates/ldap_custom.conf
```

### Deploy aerospike with 2 nodes and ldap template

```bash
$ aerolab make-cluster -n ldap -c 2 -o templates/ldap_custom.conf -f features.conf -m mesh
$ rm templates/ldap_custom.conf
```

### Test that the connection is working
```bash
$ aerolab node-attach -n ldap -- aql --auth EXTERNAL_INSECURE -Ubadwan -Pblastoff -c "show bins"
```

### Add query-user-dn and query-user-password-file if aerospike roles permissions are needed

        ```
        query-user-dn cn=admin,dc=aerospike,dc=com
        query-user-password-file /tmp/password.txt
        ```

### Remove cluster

```bash
$ aerolab cluster-destroy -f 1 -n ldap
```

### Remove and kill the ldap server

```bash
$ docker stop ldap_server ; docker rm ldap_server
```
