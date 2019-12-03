# kafka-connect

In conjunction with aerolab, this project allows you to quickly build a container based system allowing you to set up an aerospike / kafka / kafka-connect solution. 

## Pre-requisites

Docker desktop version 17.12 or higher. This to allow use of docker-compose file format 3.5 or above.

citrusleaf credentials for enterprise downloads

## Aerospike cluster setup

You will need to set up a cluster using aerolab. Quick start is 

1. Make sure aerolab binary is in your path
2. cd to cluster-setup
3. Make sure you have a valid features.conf in this directory including the mesg-kafka-connector option
4. Make sure the citrusleaf password for www.aerospike.com downloads is set correctly in credentials.conf
4. Create the kafka-connect cluster by cd-ing to ```cluster-setup``` and running ```./make-cluster.sh```. This will make a cluster with name 'kafka-connect'
5. You can verify your cluster is available via ```aerolab aql -n kafka-connect -l 1``` - aql into 1st node in the 'kafka-connect' cluster
6. ```aerolab cluster-list``` to see cluster detail 

This sets up a cluster including exposing the kafka connector via xdr.

```
xdr {
        enable-xdr true
        enable-change-notification true
        xdr-digestlog-path /opt/aerospike/digestlog 100G
        xdr-compression-threshold 1000
        xdr-info-port 3004
        datacenter kafka-connect {
                dc-type http
                http-version v2
                http-url http://kafka-connect:8080/aerospike/kafka/publish
        }
}
```

and replicating the 'device' namespace to it

```
namespace device {
	...
	xdr-remote-datacenter kafka-connect
```

## kafka-connect start

Run ```./kafka-connect-setup.sh``` 

This will set up Kafka, Zookeeper and kafka-connect containers. 

Verify with ```docker container list | grep kafka```. Sample output

```
e68e5fefe18e        wurstmeister/zookeeper      "/bin/sh -c '/usr/sb…"   6 minutes ago       Up 6 minutes        22/tcp, 2888/tcp, 3888/tcp, 0.0.0.0:2181->2181/tcp   kafka-connect_zookeeper_1
dcc0ff600cd7        aerospike/kafka-connect     "/bin/sh -c 'service…"   6 minutes ago       Up 6 minutes                                                             kafka-connect_kafka-connect_1
dd029138a84c        kafka-connect_kafka         "start-kafka.sh"         6 minutes ago       Up 6 minutes        0.0.0.0:32809->9092/tcp                              kafka-connect_kafka_1
b983cc96d42c        aero-ubuntu_18.04:4.6.0.4   "/bin/bash"              About an hour ago   Up About an hour                                                         aero-kafka-connect_3
0130c37f89af        aero-ubuntu_18.04:4.6.0.4   "/bin/bash"              About an hour ago   Up About an hour                                                         aero-kafka-connect_2
8d9b1b8dbc07        aero-ubuntu_18.04:4.6.0.4   "/bin/bash"              About an hour ago   Up About an hour                                                         aero-kafka-connect_1
```

In particular, the kafka-connect container is configured to pass messages to the *aerospike* topic on the *kafka* service

```
connector:
  format: json
  routing:
    topic: aerospike
    mode: static
  kafka:
    bootstrapServers:
      - kafka:9092
    producerPoolSize: 1
  ticker:
    enabled: true
    period: 10s
```
## Demonstration

Insert some data to your cluster ...

1. Connect to a Kafka container ```docker exec -it kafka-connect_kafka_1 bash```
2. Open a consumer for the aerospike topic - ```$KAFKA_HOME/bin/kafka-console-consumer.sh --bootstrap-server kafka-connect_kafka_1:9092 --topic aerospike```. Leave this window open
3. From a fresh terminal window, connect to Aerospike ```aerolab aql -n kafka-connect -l 1```
4. Insert a record to the 'device' namespace ```insert into device.test(PK,value) values(1,1)```. In your kafka consumer window you should see 
```
{"msg":"write","key":["device","test","9fhvPUhVrf/o3vmg2QL6xx9Xi48=",null],"gen":0,"exp":0,"lut":0,"bins":[{"name":"value","type":"int","value":1}]}
```

## Teardown

```./kafka-connect-teardown.sh``` removes Kafka / kafka-connect and Zookeeper containers. It also removes the aero-kafka-connect cluster from the kafka-connect_net docker network

Remove your cluster using ```aerolab cluster-stop -n kafka-connect``` followed by ```aerolab cluster-destroy -n kafka-connect```

## Troubleshooting

To test Kafka, open up two windows, one for producing messages and one for consuming. Do this by logging into the relevant container

```docker exec -it kafka-connect_kafka_1 bash```

To consume messsages run

```$KAFKA_HOME/bin/kafka-console-consumer.sh --bootstrap-server kafka-connect_kafka_1:9092 --topic aerospike```

and to send messages, in the producer window run

```$KAFKA_HOME/bin/kafka-console-producer.sh --bootstrap-server kafka-connect_kafka_1:9092 --topic aerospike```

If you type your message into the producer window, followed by enter, you will see it appear in the consumer window.

To test kafka-connect is working, keep the consumer window above open and log into the *kafka-connect* container and run a test script

```docker exec -it kafka-connect_kafka-connect_1 bash

cd /opt/aerospike-kafka/bin
./test.sh
```

You should see

```
{"msg":"write","key":["test","tests","i2Ejrq8uPFTLpwAn2TI2YcaybfQ=","key"],"gen":0,"exp":0,"lut":0,"bins":[{"name":"i","type":"int","value":42},{"name":"s","type":"str","value":"foo"},{"name":"f","type":"float","value":1.99}]}
```

appear in your consumer window


