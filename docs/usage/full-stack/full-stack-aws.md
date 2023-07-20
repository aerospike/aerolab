[Docs home](../../../README.md)

# Full server-client stack with AMS monitoring example for AWS


### Define variables

```
cluster_name="my-cluster"
client_name="my-client"
ams_name="ams"
namespace="test"
backend="aws"
aws_region="ca-central-1"
aws_az="ca-central-1a"
aws_instance_type_server="t3a.medium"
aws_instance_type_ams="t3a.large"
aws_instance_type_client="t3a.medium"
```

The above variables are examples. Adjust them as necessary for your AWS configuration.

### Configure backend

```
aerolab config backend -t ${backend} -r ${aws_region}
```

### Deploy a 5-node cluster

```
aerolab cluster create -c 5 -n ${cluster_name} -I ${aws_instance_type_server} -U ${aws_az}
```

### Add exporter to the cluster to monitor in AMS

```
aerolab cluster add exporter -n ${cluster_name}
```

## Deploy AMS monitoring stack

### Using AWS or Docker where the containers are directly accessible from the host

```
aerolab client create ams -n ${ams_name} -s ${cluster_name} -I ${aws_instance_type_ams} -U ${aws_az}
```

### Deploy 5 tools machines for `asbenchmark`

```
aerolab client create tools -n ${client_name} -c 5 -I ${aws_instance_type_client} -U ${aws_az}
```

### Add Promtail to clients to push `asbenchmark` logs to AMS stack

```
aerolab client configure tools -l all -n ${client_name} --ams ${ams_name}
```

### Get IP address of AMS node to connect to Grafana

```
aerolab client list
```

Note down the IP address of the `AMS` machine. Navigate in your browser to this
address on port 3000, for example `http://1.2.3.4:3000` (or `http://127.0.0.1:3000`
if deploying AMS with Docker's port forwarding).

### Run `asbench`

```
aerolab client attach -n ${client_name} -l all --detach -- /bin/bash -c "run_asbench ...asbench-params..."
```

### Stop `asbench`

```
aerolab client attach -n ${client_name} -l all -- pkill -9 asbench
```

### Cleanup

```
aerolab cluster destroy -f -n ${cluster_name}
aerolab client destroy -f -n ${client_name}
aerolab client destroy -f -n ${ams_name}
```

## Example `asbench` commands

### Get IP address of a cluster node

```
NODEIP=$(aerolab cluster list -i |grep ${cluster_name} |head -1 |egrep -o 'ext_ip=.*' |awk -F'=' '{print $2}')
echo "Seed: ${NODEIP}"
```

### Insert data, different set per client

```
aerolab client attach -n ${client_name} -l all --detach -- /bin/bash -c "run_asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${namespace} -s \$(hostname) --latency -b testbin -K 0 -k 1000000 -z 16 -t 0 -o I1 -w I --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2"
```

### Check if `asbench` is running

```
aerolab client attach -n ${client_name} -l all -- ps -ef |grep asbench
```

### Stop `asbench`

```
aerolab client attach -n ${client_name} -l all -- pkill -9 asbench
```


### Run a read-update load, different set per client

```
aerolab client attach -n ${client_name} -l all --detach -- /bin/bash -c "run_asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${namespace} -s \$(hostname) --latency -b testbin -K 0 -k 1000000 -z 16 -t 86400 -g 1000 -o I1 -w RU,80 --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2"
```
