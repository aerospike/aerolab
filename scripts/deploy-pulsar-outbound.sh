#!/bin/bash

# Launch Pulsar standalone container.

docker run -it -d \
--name=pulsar \
-p 6650:6650 \
-p 8080:8080 \
--mount source=pulsardata,target=/pulsar/data \
--mount source=pulsarconf,target=/pulsar/conf \
apachepulsar/pulsar:2.8.1 \
bin/pulsar standalone || exit 1

sleep 10

PULSAR_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' pulsar)

# pulsar-outbound setup

cat <<'EOF' > $HOME/Downloads/aerospike-pulsar-outbound.yml
# port that the connector runs on
service:
  port: 8080
  # Aerospike Enterprise Server >= 5.0
  manage:
    address: 0.0.0.0
    port: 8902


# pulsar client configuration
client-configuration:
  serviceUrl : pulsar://$PULSAR_IP:6650
  authPluginClassName : null
  authParams : null
  operationTimeoutMs : 30000
  statsIntervalSeconds : 60
  numIoThreads : 1
  numListenerThreads : 1
  connectionsPerBroker : 1
  useTcpNoDelay : true
  useTls : false
  tlsTrustCertsFilePath : null
  tlsAllowInsecureConnection : false
  tlsHostnameVerificationEnable : false
  concurrentLookupRequest : 5000
  maxLookupRequest : 50000
  maxNumberOfRejectedRequestPerConnection : 50
  keepAliveIntervalSeconds : 30
  connectionTimeoutMs : 10000
  requestTimeoutMs : 60000
  initialBackoffIntervalNanos : 100000000
  maxBackoffIntervalNanos : 60000000000

# pulsar topic wise producer configuration
topic-wise-producer-props:
  'persistent://public/default/myTopic':
    topicName: 'persistent://public/default/myTopic'
    producerName: 'producer_persistent://public/default/myTopic'
    sendTimeoutMs: 2000

# log location if not stdout
logging:
  file: /var/log/aerospike-pulsar-outbound/aerospike-pulsar-outbound.log

# for the configurations below see the note above on specificity.

# one of json, flat-json, message-pack, or avro
format:
  mode: json

# a list of transformations and mappings.
bin-transforms:
  map:
    yellow: red
    transforms: # will be done in order
     - uppercase

routing:
  mode: static
  destination: default

namespaces:
  test:
    routing:
      mode: static
      destination: test
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

docker run -d -p 8081:8080 --name=as-pulsar-outbound  -v $HOME/Downloads/aerospike-pulsar-outbound.yml:/etc/aerospike-pulsar-outbound/aerospike-pulsar-outbound.yml aerospike/aerospike-pulsar-outbound:2.1.0 || exit 2

sleep 5

PULSAR_OUTBOUND_IP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' as-pulsar-outbound)
echo "The pulsar-outbound IP is $PULSAR_OUTBOUND_IP"

