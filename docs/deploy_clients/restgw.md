[Docs home](../../README.md)

# Deploy a Rest Gateway


Use the [Aerospike REST Gateway](https://developer.aerospike.com/client/rest)
to communicate with an Aerospike cluster via RESTful requests.

1. Create a test cluster with three nodes

```
aerolab cluster create -n bob -c 3
```

2. Insert test data into namespace `test`

```
aerolab data insert -z 10000 -u 5 -n bob -m test
```

3. Deploy a Rest Gateway client

```
aerolab client create rest-gateway -n myrest -C bob
```

4. Attach to the Rest Gateway machine and execute a scan

```
aerolab attach client -n myrest
$ curl http://localhost:8080/v1/scan/test
```

### Notes

* To adjust the seed IP or connect with a username and password at a later time,
  use the `aerolab client configure rest-gateway` command. Append `help` at the
  end of the command for usage information.
* You can adjust all settings, including the startup, by changing the startup script
  on the Rest Gateway machine, located at `/opt/autoload/01-restgw.sh`.