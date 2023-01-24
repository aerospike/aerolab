# Full server-client stack with AMS monitoring example

## Define variables

cluster_name="robert"
client_name="glonek"
ams_name="ams"
namespace="test"

## Deploy a 5-node cluster

```
aerolab cluster create -c 5 -n ${cluster_name}
```

## Add exporter to the cluster to monitor in AMS

```
aerolab cluster add exporter -n ${cluster_name}
```

## Deploy AMS monitoring stack itself

```
aerolab client create ams -n ${ams_name} -s ${cluster_name}
```

## Deploy 5 tools machines for asbenchmark

```
aerolab client create tools -n ${client_name} -c 5
```

## Add promtail to clients to push asbenchmark logs to AMS stack

```
aerolab client configure tools -l all -n ${client_name} --ams ${ams_name}
```

## Run asbench

```
aerolab client attach -n ${client_name} -l all --detach -- run_asbench ...asbench-params...
```

### Example asbench commands

#### Get IP of a cluster node

```
NODEIP=$(aerolab cluster list -j |grep -A7 ${cluster_name} |grep IpAddress |head -1 |egrep -o '([0-9]{1,3}\.){3}[0-9]{1,3}')
echo "Seed: ${NODEIP}"
```

#### Insert data, different set per client

```
aerolab client attach -n ${client_name} -l all --detach -- run_asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${namespace} -s \$(hostname) --latency -b testbin -K 0 -k 1000000 -z 16 -t 0 -o I1 -w I --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2
```

#### Run a read-update load, different set per client

```
aerolab client attach -n ${client_name} -l all --detach -- run_asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${namespace} -s \$(hostname) --latency -b testbin -K 0 -k 1000000 -z 16 -t 86400 -g 1000 -o I1 -w RU,80 --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2
```
