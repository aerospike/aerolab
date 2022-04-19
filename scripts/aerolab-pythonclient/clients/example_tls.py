#!/usr/bin/python3
# import the module
from __future__ import print_function
import aerospike
from aerospike import exception as ex
import time

#######################################################################################
## This code authenticates with the database, writes a record and then reads it back ##
#######################################################################################

# Modify CLUSTERIP to be the IP of one of your database nodes

# Configure the client CLUSTERIP:4333
config = {
  'hosts': [ ('CLUSTERIP', 4333, 'server1') ],
  'policies': {
    'timeout': 1000,
    'auth_mode': aerospike.AUTH_EXTERNAL
  },
  'tls': {
    'enable': True,

    # system-wide CA trust.
    'cafile': '/root/certs/local/rootCA.pem',

    # For mTLS the client must present it's public certificate to the
    # server during the TLS handshake. This can be removed if Aerospike
    # Server is not configured for mutual TLS (tls-authenticate-client = false)
    'certfile': '/root/certs/output/client1.pem',

    # For mTLS the client will need the private key to encrypt messages
    # sent to the server during the TLS handshake. This can be removed
    # if Aerospike Server is not configured for mutual TLS
    # (tls-authenticate-client = false)
    'keyfile': '/root/certs/output/client1.key',

    # The 'cipher_suite' property is optional, however, it is recommended
    # to provide a list of valid cipher suites to ensure less secure or
    # poor-performing algorithms are not available. The cipher suites
    # can be specified in the 'cipher_suite' as shown below, they can be
    # specified in the Aerospike configuration using the 'cipher-suite'
    # directive, or both.
    'cipher_suite': 'ECDHE-RSA-AES256-GCM-SHA384:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES128-GCM-SHA256:AES256-GCM-SHA384:AES128-GCM-SHA256'
  }
}

# Create a client and connect it to the cluster
print("Connecting/Authenticating")
try:
  client = aerospike.client(config).connect('badwan','blastoff')
except ex.NotAuthenticated as e:
  import sys
  print("Not Authenticated : {0} [{1}] - {2}".format(e.msg, e.code, config))
  sys.exit(1)
except Exception as e:
  import sys
  print("Exception : {0} [{1}] - {2}".format(e.msg, e.code, config))
  sys.exit(1)


print("Ready to Read/Write")
var=input("Press Enter")
print("")
key = ('test', 'demo', 'key1')

# Records are addressable via a tuple of (namespace, set, key)
print("Writing Key : %s" %(key,))
try:
  # Write a record
  client.put(key, { 'name': 'John Doe', 'age': 50 })
except Exception as e:
  import sys
  print("Exception : {0} [{1}]".format(e.msg, e.code))
  time.sleep(1)
  sys.exit(1)

print("")

# Read a record
print("Reading Key : %s" %(key,))
try:
  (key, metadata, record) = client.get(key)
except Exception as e:
  import sys
  print("Exception : {0} [{1}]".format(e.msg, e.code))
  time.sleep(1)
  sys.exit(1)

print("Record : %s" % (record,))
#  time.sleep(.1)

# Close the connection to the Aerospike cluster
client.close()