#!/bin/bash

# Launch single node kafka cluster using docker-compose. docker-compose should be installed on the system.

cat <<'EOF' > docker-compose.yml
version: '2'
services:
  zookeeper:
    image: confluentinc/cp-zookeeper:latest
    container_name: zookeeper
    environment:
      ZOOKEEPER_CLIENT_PORT: 2181
      ZOOKEEPER_TICK_TIME: 2000
    ports:
      - 22181:2181
    networks:
      - kafka_default
  
  kafka:
    image: confluentinc/cp-kafka:latest
    container_name: kafka
    depends_on:
      - zookeeper
    ports:
      - 29092:29092
    environment:
      KAFKA_BROKER_ID: 1
      KAFKA_ZOOKEEPER_CONNECT: zookeeper:2181
      KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092,PLAINTEXT_HOST://localhost:29092
      KAFKA_LISTENER_SECURITY_PROTOCOL_MAP: PLAINTEXT:PLAINTEXT,PLAINTEXT_HOST:PLAINTEXT
      KAFKA_INTER_BROKER_LISTENER_NAME: PLAINTEXT
      KAFKA_OFFSETS_TOPIC_REPLICATION_FACTOR: 1
    networks:
      - kafka_default

networks:
  kafka_default:
    name: kafka_default

EOF

docker-compose  up -d || exit 1

sleep 10

docker-compose logs kafka | grep -i started


# kafka-outbound setup

cat <<'EOF' > $HOME/Downloads/aerospike-kafka-outbound.yml
# port that the connector runs on
service:
  port: 8080

# kafka producer props https://kafka.apache.org/23/javadoc/org/apache/kafka/clients/producer/ProducerConfig.html
producer-props:
  bootstrap.servers:
    - kafka:9092

# log location if not stdout
logging:
  file: /var/log/aerospike-kafka-outbound/aerospike-kafka-outbound.log

# one of json, flat-json, message-pack, avro, or kafka
format:
  mode: json

# a list of transformations and mappings.
bin-transforms:
  map:
    yellow: red
    transforms: //will be done in order
      - uppercase

routing:
  mode: static
  destination: default

namespaces:
  users:
    routing:
      mode: static
      destination: users
    format:
      mode: flat-json
      metadata-key: metadata
    sets:
      premium:
        routing:
          mode: static
          destination: premium
        bin-transforms:
          map:
            gold: platinum

EOF

docker run  -d -p 8080:8080  --name as-kafka-outbound --network kafka_default -v $HOME/Downloads/aerospike-kafka-outbound.yml:/etc/aerospike-kafka-outbound/aerospike-kafka-outbound.yml aerospike/aerospike-kafka-outbound:4.0.0 || exit 2

sleep 5

