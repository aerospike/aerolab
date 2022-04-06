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

print("Ready")
var=input("Press Enter")
print("Going")

# Records are addressable via a tuple of (namespace, set, key)
i=0
while i < 10000:
  key = ('test', 'demo', 'foorun3'+str(i))
  print(key)

  try:
    # Write a record
    client.put(key, { 'name': 'John Doe', 'age': i })
  except Exception as e:
    import sys
    print("error: {0}".format(e), file=sys.stderr)
    time.sleep(1)
    continue

  # Read a record
  try:
    (key, metadata, record) = client.get(key)
  except Exception as e:
    import sys
    print("error: {0}".format(e), file=sys.stderr)
    time.sleep(1)
    continue

  print(record)
#  time.sleep(.1)
  i +=1

# Close the connection to the Aerospike cluster
client.close()
