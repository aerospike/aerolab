## Deploying KAFKA OUTBOUND CONNECTOR on docker

### Install docker-compose
On Ubuntu
```bash
$ sudo curl -L "https://github.com/docker/compose/releases/download/1.27.4/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
$ sudo chmod +x /usr/local/bin/docker-compose
```

For different OS distributions, follow [install](https://docs.docker.com/compose/install/) link.

### Deploy kafka cluster and kafka-outbound-connector on docker

Get and run [deploy-kafka-outbound.sh](/scripts/deploy-kafka-outbound.sh) file.
```bash
$ ./scripts/deploy-kafka-outbound.sh
```

### Change kafka-outbound.conf to point aerospike at the kafka-outbound-connector container
```bash
$ KAFKA_OUTBOUND_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' as-kafka-outbound)
$ sed "s/KAFKA_OUTBOUND_IP/$KAFKA_OUTBOUND_IP/g" templates/kafka-outbound.conf > templates/kafka-outbound-custom.conf
```

### Deploy aerospike with 1 node and kafka-outbound-connector template

```bash
$ aerolab make-cluster -n kafka-dc1  -o templates/kafka-outbound-custom.conf
$ rm templates/kafka-outbound-custom.conf
$ docker network connect kafka_default aero-kafka-dc1_1
```


### Test that XDR shipping records to kafka

Insert record in test namespace on aerospike server
```bash
$ aerolab node-attach -n kafka-dc1 -- aql -c "insert into test(PK,a,b) values('2','aaa',124);"
$ aerolab node-attach -n kafka-dc1 -- aql -c "insert into test(PK,a,b) values('3','aaa',124);"
```

Verify the records on kafka cluster
```bash
$ docker exec -it kafka kafka-console-consumer --bootstrap-server kafka:29092  --topic default --from-beginning
{"msg":"write","key":["test",null,"2EpRiyMMVhOf6nV4XBr/PFofZBY=",null],"gen":1,"exp":0,"lut":1635778671921,"bins":[{"name":"a","type":"str","value":"aaa"},{"name":"b","type":"int","value":124}]}
{"msg":"write","key":["test",null,"DXMkHIhKgkmtXoKOKXTBsIppBNw=",null],"gen":1,"exp":0,"lut":1635778815179,"bins":[{"name":"a","type":"str","value":"aaa"},{"name":"b","type":"int","value":124}]}
```

Check logs on kafka outbound connector
```bash
$ docker exec -it as-kafka-outbound tail -f /var/log/aerospike-kafka-outbound/aerospike-kafka-outbound.log
```
### Remove cluster

```bash
$ aerolab cluster-destroy -f 1 -n kafka-dc1
```

### Remove and kill the kafka setup

```bash
$ docker stop as-kafka-outbound ; docker rm as-kafka-outbound
$ docker stop kafka ; docker rm kafka
$ docker stop zookeeper ; docker rm zookeeper
```
