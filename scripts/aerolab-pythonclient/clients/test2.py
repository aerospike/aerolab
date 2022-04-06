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

i=0
key = ('test', 'demo', 'foorun39')
print("Create first record")
while i < 1:
  err=0
  try:
    client = aerospike.client(config).connect('badwan','blastoff')
  except:
    err=1
    import sys
    print("failed to connect to the cluster with", config['hosts'])
  if err == 0 :
    try:
      client.put(key, { 'name': 'John Doe', 'age': 32 })
      client.close()
#      print(record)
    except Exception as e:
      import sys
      print("error: {0}".format(e), file=sys.stderr)
print("Create rest of records")
# Records are addressable via a tuple of (namespace, set, key)
i=1
while i < 10000:
  # Create a client and connect it to the cluster
  try:
    client = aerospike.client(config).connect('badwan','blastoff')
  except:
    import sys
    print("failed to connect to the cluster with", config['hosts'])
    sys.exit(1)

  key = ('test', 'demo', 'foorun3'+str(i))
  print(key)

  try:
    # Write a record
    client.put(key, { 'name': 'John Doe', 'age': 32 })
  except Exception as e:
    import sys
    print("error: {0}".format(e), file=sys.stderr)

  # Read a record
  try:
    (key, metadata, record) = client.get(key)
    client.close()
  except Exception as e:
    import sys
    print("error: {0}".format(e), file=sys.stderr)

  print(record)
#  time.sleep(0.1)
  i +=1

# Close the connection to the Aerospike cluster
client.close()
