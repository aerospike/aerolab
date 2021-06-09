CONFIG_FILE=kafka-connect.conf
CLUSTER_NAME=kafka-connect
VERSION=5.6.0.5

aerolab make-cluster -n $CLUSTER_NAME -m mesh -c 3 -o `pwd`/$CONFIG_FILE -f `pwd`/features.conf -v $VERSION
