#!/usr/bin/python3
# import the module
from __future__ import print_function
import aerospike
import time

# Configure the client CLUSTERIP:3000
config = {
  'hosts': [ ('CLUSTERIP', 3000) ],
  'policies': {
      'timeout': 1000,
      'auth_mode': aerospike.AUTH_EXTERNAL_INSECURE
  }
}

# Create a client and connect it to the cluster
try:
  client = aerospike.client(config).connect('badwan','blastoff')
except:
  import sys
  print("failed to connect to the cluster with", config['hosts'])
  sys.exit(1)

# Records are addressable via a tuple of (namespace, set, key)
i=1
key = ('test', 'demo', 'foorun3'+str(i))
print(key)

# Read a record
try:
  (key, metadata, record) = client.get(key)
except Exception as e:
  import sys
  print("error: {0}".format(e), file=sys.stderr)

print(record)
time.sleep(0.1)

command="sets/test"
response = client.info_all(command)

print(response)
# Close the connection to the Aerospike cluster
client.close()