## Deploying PULSAR OUTBOUND CONNECTOR on docker

### Deploy pulsar standalone cluster and plsar-outbound-connector on docker

Get and run [deploy-pulsar-outbound.sh](/scripts/deploy-pulsar-outbound.sh) file.
```bash
$ ./scripts/deploy-pulsar-outbound.sh
```

### Change pulsar-outbound.conf to point aerospike at the pulsar-outbound-connector container
```bash
$ PULSAR_OUTBOUND_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' as-pulsar-outbound)
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
----- got message -----
key:[eyJuYW1lc3BhY2UiOiJ0ZXN0IiwiZGlnZXN0IjoidGNTbXZzK3pPWVlGdnQyVGY1cnBRZERPNGQ0PSJ9], properties:[], content:{"metadata":{"namespace":"test","digest":"tcSmvs+zOYYFvt2Tf5rpQdDO4d4=","msg":"write","gen":1,"lut":1636407336859,"exp":0},"A":"aaa","B":124}
----- got message -----
key:[eyJuYW1lc3BhY2UiOiJ0ZXN0IiwiZGlnZXN0IjoiMkVwUml5TU1WaE9mNm5WNFhCci9QRm9mWkJZPSJ9], properties:[], content:{"metadata":{"namespace":"test","digest":"2EpRiyMMVhOf6nV4XBr/PFofZBY=","msg":"write","gen":1,"lut":1636407352300,"exp":0},"A":"aaa","B":124}
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
# Clean docker volume to clean old data
$ docker volume rm pulsarconf ; docker volume rm pulsardata
```
