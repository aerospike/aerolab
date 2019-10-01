#!/bin/bash

docker run --name ldap_server --env LDAP_ORGANISATION="aerospike" --env LDAP_DOMAIN="aerospike.com"  --detach osixia/openldap:1.2.0 || exit 1

ready=255
while [ $ready -ne 0 ]
do
docker exec -it ldap_server ldapsearch -x -b dc=aerospike,dc=com -D "cn=admin,dc=aerospike,dc=com" -w admin
ready=$?
done

cat <<'EOF' > access.ldif
dn: olcDatabase={1}mdb,cn=config
add: olcAccess
olcAccess: {0} to * by * read
EOF

docker cp access.ldif  ldap_server:/tmp/access.ldif || exit 2

docker exec -it ldap_server ldapmodify -H ldapi:// -Y EXTERNAL -f /tmp/access.ldif || exit 3

cat <<'EOF' > people.ldif
dn: ou=People,dc=aerospike,dc=com
objectClass: organizationalUnit
ou: People
EOF

docker cp people.ldif ldap_server:/tmp/people.ldif || exit 4

docker exec -it ldap_server ldapadd -x -w admin -D "cn=admin,dc=aerospike,dc=com" -f /tmp/people.ldif || exit 5

cat <<'EOF' > badwan.ldif
dn: uid=badwan,ou=People,dc=aerospike,dc=com
objectClass: top
objectClass: account
objectClass: posixAccount
objectClass: shadowAccount
cn: svc.aeros.cam.devp1
uid: svc.aeros.cam.devp1
uidNumber: 834
gidNumber: 100
homeDirectory: /home/badwan
loginShell: /bin/bash
gecos: badwan
userPassword: blastoff
shadowLastChange: 0
shadowMax: 0
shadowWarning: 0
EOF

docker cp badwan.ldif ldap_server:/tmp/badwan.ldif || exit 6

docker exec -it ldap_server ldapadd -x -w admin -D "cn=admin,dc=aerospike,dc=com" -f /tmp/badwan.ldif || exit 7

cat <<'EOF' > read-write-udf.ldif
dn: cn=read-write-udf,dc=aerospike,dc=com
objectClass: top
objectClass: posixGroup
gidNumber: 680
EOF

docker cp read-write-udf.ldif ldap_server:/tmp/read-write-udf.ldif || exit 8

docker exec -it ldap_server ldapadd -x -w admin -D "cn=admin,dc=aerospike,dc=com" -f /tmp/read-write-udf.ldif || exit 9

cat <<'EOF' > modify.ldif
dn: cn=read-write-udf,dc=aerospike,dc=com
changetype: modify
add: memberuid
memberuid: badwan
EOF

docker cp modify.ldif ldap_server:/tmp/modify.ldif || exit 10

docker exec -it ldap_server ldapmodify -x -w admin -D "cn=admin,dc=aerospike,dc=com" -f /tmp/modify.ldif || exit 11

docker exec -it ldap_server ldapsearch -x -b dc=aerospike,dc=com -D "cn=admin,dc=aerospike,dc=com" -w admin || exit 12

echo

echo "User: badwan; Password: blastoff"

echo -e "To kill the ldap server: docker stop ldap_server; docker rm ldap_server"

echo "IP of ldap server: "
docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ldap_server
