# How to manually create intermediate signed SSL certs

## This tutorial assumes you have aerolab 2.50+, if not, update!

## let's make a container and attach

```
rglonek@Roberts-MacBook-Pro ~ % aerolab deploy-container -n ssl
Jul 10 11:27:21+0000 AERO-LAB[45933]: INFO     Performing sanity checks, checking if docker/lxc is running and accessible
Jul 10 11:27:21+0000 AERO-LAB[45933]: INFO     Checking if version template already exists
Jul 10 11:27:21+0000 AERO-LAB[45933]: INFO     Starting deployment
Jul 10 11:27:22+0000 AERO-LAB[45933]: INFO     Done
rglonek@Roberts-MacBook-Pro ~ % aerolab node-attach -n ssl
```

## preparation steps

```
root@fa080696bdf3:/# mkdir CA
root@fa080696bdf3:/# cd CA
root@fa080696bdf3:/CA# mkdir private newcerts
root@fa080696bdf3:/CA# touch index.txt
root@fa080696bdf3:/CA# echo "01" > serial
```

## make config for openssl

```
root@fa080696bdf3:/CA# cat <<'EOF' > openssl.cnf
#
# OpenSSL configuration file.
#

# Establish working directory.

dir			= .

[ req ]
default_bits  	    = 1024		# Size of keys
default_keyfile     = key.pem		# name of generated keys
default_md          = sha256		# message digest algorithm
string_mask         = nombstr		# permitted characters
distinguished_name  = req_distinguished_name
req_extensions      = v3_req

[ req_distinguished_name ]
# Variable name		        Prompt string
#----------------------   ----------------------------------
0.organizationName        = Organization Name (company)
organizationalUnitName    = Organizational Unit Name (department, division)
emailAddress              = Email Address
emailAddress_max          = 40
localityName              = Locality Name (city, district)
stateOrProvinceName       = State or Province Name (full name)
countryName               = Country Name (2 letter code)
countryName_min           = 2
countryName_max           = 2
commonName                = Common Name (hostname, IP, or your name)
commonName_max            = 64

# Default values for the above, for consistency and less typing.
# Variable name			  Value
#------------------------------	  ------------------------------
0.organizationName_default         = Aerospike Inc
organizationalUnitName_default     = operations
emailAddress_default               = operations@aerospike.com
localityName_default               = Bangalore
stateOrProvinceName_default	   = Karnataka
countryName_default		   = IN
commonName_default                 = harvey 

[ v3_ca ]
basicConstraints	= CA:TRUE
subjectKeyIdentifier	= hash
authorityKeyIdentifier	= keyid:always,issuer:always

[ v3_req ]
basicConstraints	= CA:FALSE
subjectKeyIdentifier	= hash

[ ca ]
default_ca		= CA_default

[ CA_default ]
serial			= $dir/serial
database		= $dir/index.txt
new_certs_dir		= $dir/newcerts
certificate		= $dir/cacert.pem
private_key		= $dir/private/cakey.pem
default_days		= 365
default_md		= sha256
preserve		= no
email_in_dn		= no
nameopt			= default_ca
certopt			= default_ca
policy			= policy_match

[ policy_match ]
countryName		= match
stateOrProvinceName	= match
organizationName	= match
organizationalUnitName	= optional
commonName		= supplied
emailAddress		= optional
EOF
```

## make CA

```
root@fa080696bdf3:/CA# openssl req -new -nodes -x509 -extensions v3_ca -keyout private/cakey.pem -out cacert.pem -days 3650 -config ./openssl.cnf -subj "/C=US/ST=Denial/L=Springfield/O=Dis/CN=tlsname1"
```

## make intermediate cert

```
root@fa080696bdf3:/CA# openssl genrsa -out intermediate1.key 8192
root@fa080696bdf3:/CA# openssl req -batch -config ./openssl.cnf -sha256 -new -key intermediate1.key -out intermediate1.csr -subj "/C=US/ST=Denial/L=Springfield/O=Dis/CN=tlsname1/"
root@fa080696bdf3:/CA# openssl ca -batch -config ./openssl.cnf -notext -in intermediate1.csr -out intermediate1.crt
```

## prepare new structure for using intemediate as CA - same steps as before, just different dir

```
root@fa080696bdf3:/CA# cd /
root@fa080696bdf3:/# mkdir INT
root@fa080696bdf3:/# cd INT
root@fa080696bdf3:/INT# mkdir private newcerts
root@fa080696bdf3:/INT# touch index.txt
root@fa080696bdf3:/INT# echo "01" > serial
```

## create new openssl.cnf - warn - we are changing a few lines here from previous one - i.e. the path to CA as we are using intermediate now as CA

```
root@fa080696bdf3:/INT# cat <<'EOF' > openssl.cnf
#
# OpenSSL configuration file.
#

# Establish working directory.

dir			= .

[ req ]
default_bits  	    = 1024		# Size of keys
default_keyfile     = key.pem		# name of generated keys
default_md          = sha256		# message digest algorithm
string_mask         = nombstr		# permitted characters
distinguished_name  = req_distinguished_name
req_extensions      = v3_req

[ req_distinguished_name ]
# Variable name		        Prompt string
#----------------------   ----------------------------------
0.organizationName        = Organization Name (company)
organizationalUnitName    = Organizational Unit Name (department, division)
emailAddress              = Email Address
emailAddress_max          = 40
localityName              = Locality Name (city, district)
stateOrProvinceName       = State or Province Name (full name)
countryName               = Country Name (2 letter code)
countryName_min           = 2
countryName_max           = 2
commonName                = Common Name (hostname, IP, or your name)
commonName_max            = 64

# Default values for the above, for consistency and less typing.
# Variable name			  Value
#------------------------------	  ------------------------------
0.organizationName_default         = Aerospike Inc
organizationalUnitName_default     = operations
emailAddress_default               = operations@aerospike.com
localityName_default               = Bangalore
stateOrProvinceName_default	   = Karnataka
countryName_default		   = IN
commonName_default                 = harvey 

[ v3_ca ]
basicConstraints	= CA:TRUE
subjectKeyIdentifier	= hash
authorityKeyIdentifier	= keyid:always,issuer:always

[ v3_req ]
basicConstraints	= CA:FALSE
subjectKeyIdentifier	= hash

[ ca ]
default_ca		= CA_default

[ CA_default ]
serial			= $dir/serial
database		= $dir/index.txt
new_certs_dir		= $dir/newcerts
certificate		= $dir/intermediate.pem
private_key		= $dir/private/intermediatekey.pem
default_days		= 365
default_md		= sha256
preserve		= no
email_in_dn		= no
nameopt			= default_ca
certopt			= default_ca
policy			= policy_match

[ policy_match ]
countryName		= match
stateOrProvinceName	= match
organizationName	= match
organizationalUnitName	= optional
commonName		= supplied
emailAddress		= optional
EOF
```

## copy over the intermediate to new path

```
root@fa080696bdf3:/INT# cp /CA/intermediate1.crt /INT/intermediate.pem
root@fa080696bdf3:/INT# cp /CA/intermediate1.key /INT/private/intermediatekey.pem
```

## make and sign new client cert request using intermediate

```
root@fa080696bdf3:/INT# openssl req -new -nodes -out req.pem -config ./openssl.cnf -subj "/C=US/ST=Denial/L=Springfield/O=Dis/CN=tlsname1"
root@fa080696bdf3:/INT# openssl ca -batch -out cert.pem -config ./openssl.cnf -infiles req.pem
```

## result:

cert | path
--- | ---
CA cert | /CA/cacert.pem
CA key | /CA/private/cakey.pem
intermediate cert | /INT/intermediate.pem
intermediate key | /INT/private/intermediatekey.pem
client cert | /INT/cert.pem
client key | /INT/key.pem

The CA signed the intermediate and the intermediate signed the client.

The process is pretty much like you would normally do: generate CA and then generate client cert. With added step - you then use the client cert in a new structure like a CA cert and use that to sign the next client cert. The one in the middle becomes intermediate.

And so lives certificate chaining. :)

## verify the certs

root@fa080696bdf3:/INT# openssl x509 -noout -text -in cert.pem
root@fa080696bdf3:/INT# openssl x509 -noout -text -in intermediate.pem 
root@fa080696bdf3:/INT# openssl x509 -noout -text -in /CA/cacert.pem

## ca cert chaining

But, without a valid root to trust, the chain will fail. Applications typically require a trusted chain for this to work (i.e. the CA and all intermediates). In this case, you can create a valid chain (yes, you can chain certificates).

```
root@fa080696bdf3:/INT# cat /INT/intermediate.pem /CA/cacert.pem > ca-chain.pem
```

and verify:

```
root@fa080696bdf3:/INT# openssl x509 -noout -text -in ca-chain.pem
```

Now, you should use `ca-chain.pem` for the ca certificate, and use the `cert.pem` as client and `key.pem` as the client key cert and it should all, theoretically, work.

If in doubt, check if it works with `cacert.pem` as ca, or with `intermediate.pem` as ca. But theoretically, one should use a whole chain, i.e. `ca-chain.pem`

## because I like tables in md files

cert | path | usage
--- | --- | ---
CA cert | /CA/cacert.pem | original CA cert, theoretically client should be able to verify using just this, maybe
CA key | /CA/private/cakey.pem | the ca key, we should not need that any more (only required for signing)
intermediate cert | /INT/intermediate.pem | intermediate cert, theoretically client should be able to verify using just this, maybe
intermediate key | /INT/private/intermediatekey.pem | the intermediate ca key, we should not need that any more (only required for signing)
client cert | /INT/cert.pem | client cert, signed by the intermediate
client key | /INT/key.pem | client key, signed by the intermediate
ca-chain | /INT/ca-chain.pem | the ca chain, this one should 100% be able to verify the client certs as the valid CA chain contains both the intermediate and CA. One should use this as their ca cert, not the intermediate.pem nor the cacert.pem, theoretically.
