CONFIG_FILE=kafka-connect.conf
CLUSTER_NAME=kafka-connect
CREDENTIALS=credentials.conf
VERSION=4.6.0.4

aerolab make-cluster --config=`pwd`/$CREDENTIALS -n $CLUSTER_NAME -m mesh -c 3 -o `pwd`/$CONFIG_FILE -f `pwd`/features.conf -v $VERSION
