# Full server-client stack with AMS monitoring example for Docker

## Define variables

```
cluster_name="robert"
client_name="glonek"
ams_name="ams"
namespace="test"
```

## Deploy a 5-node cluster

```
aerolab cluster create -c 5 -n ${cluster_name}
```

## Add exporter to the cluster to monitor in AMS

```
aerolab cluster add exporter -n ${cluster_name}
```

## Deploy AMS monitoring stack itself

### Using AWS or Docker where the containers are directly accessible from the host

```
aerolab client create ams -n ${ams_name} -s ${cluster_name}
```

### Using Docker Desktop without tunneling configured

```
aerolab client create ams -n ${ams_name} -s ${cluster_name} -e 3000:3000
```

## Deploy 5 tools machines for asbenchmark

```
aerolab client create tools -n ${client_name} -c 5
```

## Add promtail to clients to push asbenchmark logs to AMS stack

```
aerolab client configure tools -l all -n ${client_name} --ams ${ams_name}
```

## Get IP of AMS node to connect to grafana

```
aerolab client list
```

Note down the IP of the `AMS` machine, and visit, in your browser, `http://IP:3000` (or `http://127.0.0.1:3000` if deploying AMS with the docker's port forwarding)

## Run asbench

```
aerolab client attach -n ${client_name} -l all --detach -- /bin/bash run_asbench ...asbench-params...
```

## Stop asbench

```
aerolab client attach -n ${client_name} -l all -- pkill -9 asbench
```

## Cleanup

```
aerolab cluster destroy -f -n ${cluster_name}
aerolab client destroy -f -n ${client_name}
aerolab client destroy -f -n ${ams_name}
```

### Example asbench commands

#### Get IP of a cluster node

```
NODEIP=$(aerolab cluster list -j |grep -A7 ${cluster_name} |grep IpAddress |head -1 |egrep -o '([0-9]{1,3}\.){3}[0-9]{1,3}')
echo "Seed: ${NODEIP}"
```

#### Insert data, different set per client

```
aerolab client attach -n ${client_name} -l all --detach -- /bin/bash -c "run_asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${namespace} -s \$(hostname) --latency -b testbin -K 0 -k 1000000 -z 16 -t 0 -o I1 -w I --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2"
```

#### Stop asbench

```
aerolab client attach -n ${client_name} -l all -- pkill -9 asbench
```


#### Run a read-update load, different set per client

```
aerolab client attach -n ${client_name} -l all --detach -- /bin/bash -c "run_asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${namespace} -s \$(hostname) --latency -b testbin -K 0 -k 1000000 -z 16 -t 86400 -g 1000 -o I1 -w RU,80 --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2"
```
