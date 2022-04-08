#!/usr/bin/python3
# import the module
from __future__ import print_function
import aerospike
import time

#######################################################################################
## This code authenticates with the database, writes a record and then reads it back ##
#######################################################################################

# Modify CLUSTERIP to be the IP of one of your database nodes


# Configure the client CLUSTERIP:3000
config = {
  'hosts': [ ('CLUSTERIP', 3000) ],
  'policies': {
      'timeout': 1000,
      'auth_mode': aerospike.AUTH_EXTERNAL_INSECURE
  }
}

# Create a client and connect it to the cluster
print("Connecting")
try:
  client = aerospike.client(config).connect()
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
