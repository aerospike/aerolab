## Deploying PULSAR OUTBOUND CONNECTOR on docker

### Deploy pulsar standalone cluster and plsar-outbound-connector on docker

Get and run [deploy-pulsar-outbound.sh](/scripts/deploy-pulsar-outbound.sh) file.
```bash
$ ./scripts/deploy-pulsar-outbound.sh
```

### Change pulsar-outbound.conf to point aerospike at the pulsar-outbound-connector container
```bash
$ PULSAR_OUTBOUND_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' pulsar)
$ sed "s/PULSAR_OUTBOUND_IP/$PULSAR_OUTBOUND_IP/g" templates/pulsar-outbound.conf > templates/pulsar-outbound-custom.conf
```

### Deploy aerospike with 1 node and pulsar-outbound-connector template

```bash
$ aerolab make-cluster -n pulsar-dc1  -o templates/pulsar-outbound-custom.conf
$ rm templates/pulsar-outbound-custom.conf
```


### Test that XDR shipping records to kafka

Insert record in test namespace on aerospike server
```bash
$ aerolab node-attach -n pulsar-dc1 -- aql -c "insert into test(PK,a,b) values('1','aaa',124);"
$ aerolab node-attach -n pulsar-dc1 -- aql -c "insert into test(PK,a,b) values('2','aaa',124);"
```

Verify the records on pulsar cluster
```bash
$ docker exec -it pulsar /pulsar/bin/pulsar-admin topics list public/default
$ docker exec -it pulsar /pulsar/bin/pulsar-client consume -s test-subscription -n 0 test
```

Check logs on pulsar outbound connector
```bash
$ docker exec -it as-pulsar-outbound tail -f /var/log/aerospike-pulsar-outbound/aerospike-pulsar-outbound.log
```
### Remove cluster

```bash
$ aerolab cluster-destroy -f 1 -n pulsar-dc1
```

### Remove and kill the pulsar setup

```bash
$ docker stop as-pulsar-outbound ; docker rm as-pulsar-outbound
$ docker stop pulsar ; docker rm pulsar
```
