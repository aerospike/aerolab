# Generate certificates

Notes:
  * This script is designed to allow you to very quickly create a self signed Certificate Authority and generate certificates<br>
    It will automatically generate server,client,admin,ldap,xdr,fabric and heartbeat certificates<br>


## First clone this repo

```bash
# Via Web
git clone https://github.com/aerospike/aerolab.git

# Via SSH
git clone git@github.com:aerospike/aerolab.git
```

## Enter the directory
```bash
cd aerolab/scripts
```

## Usage

```bash
./make-certificates.sh 
```

Certificates will be placed in ~/rootca
```
local/rootCA.pem - Root certificate
local/rootCA.key - Root certificate key

output/server1.pem - Server certificate
output/server1.key - Server certificate key
output/client1.pem - Client certificate
output/client1.key - Client certificate key
output/ldap1.pem - LDAP Server certificate
output/ldap1.key - LDAP Server certificate key
output/admin1.pem - asadm/aql test certificate
output/admin1.key - asadm/aql test certificate key
output/xdr1.pem - XDR certificate
output/xdr1.key - XDR certificate key
```
For reference, the requests used to generate certificates are placed in input

