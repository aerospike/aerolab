# Deploying a Rest Gateway

## Create a test cluster with three nodes

```
aerolab cluster create -n bob -c 3
```

## Insert test data into test namespace

```
aerolab data insert -z 10000 -u 5 -n bob -m test
```

## Deploy a rest gateway client

```
aerolab client create rest-gateway -n myrest -C bob
```

## Attach to the rest gateway machine and execute a scan

```
aerolab attach client -n myrest
$ curl http://localhost:8080/v1/scan/test
```

## Notes

* to adjust the seed IP or connect username or password at a later time, use the `aerolab client configure rest-gateway` command. Append `help` at the end of usage information
* all settings can be adjusted, including the startup, by changing the startup script on the rest gateway machine, located at `/opt/autoload/01-restgw.sh`
