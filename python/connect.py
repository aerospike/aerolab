import aerospike

config = {
    'hosts': [
        ( '172.17.0.2', 3000 )
    ],
    'policies': {
        'timeout': 10000 # milliseconds
    }
}

client = aerospike.client(config)

client.connect()

# TODO code here

client.close()
