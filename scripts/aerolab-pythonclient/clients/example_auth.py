#!/usr/bin/python3
# import the module
from __future__ import print_function
import aerospike
import time

#######################################################################################
## This code authenticates with the database, writes a record and then reads it back ##
#######################################################################################


# Configure the client CLUSTERIP:3000
config = {
  'hosts': [ ('CLUSTERIP', 3000) ],
  'policies': {
      'timeout': 1000,
      'auth_mode': aerospike.AUTH_EXTERNAL_INSECURE
  }
}

# Create a client and connect it to the cluster
print("Connecting/Authenticating")
try:
  client = aerospike.client(config).connect('badwan','blastoff')
except:
  import sys
  print("failed to connect to the cluster with", config['hosts'])
  sys.exit(1)

print("Ready to Read/Write")
var=input("Press Enter")
print("")
key = ('test', 'demo', 'key1')

# Records are addressable via a tuple of (namespace, set, key)
print("Writing Key :"+key)
try:
  # Write a record
  client.put(key, { 'name': 'John Doe', 'age': 50 })
except Exception as e:
  import sys
  print("error: {0}".format(e), file=sys.stderr)
  time.sleep(1)
  continue

print("")

# Read a record
print("Reading Key :"+key)
try:
  (key, metadata, record) = client.get(key)
except Exception as e:
  import sys
  print("error: {0}".format(e), file=sys.stderr)
  time.sleep(1)
  continue

print("Record : "+record)
#  time.sleep(.1)

# Close the connection to the Aerospike cluster
client.close()
