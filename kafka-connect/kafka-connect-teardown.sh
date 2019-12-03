CLUSTER_NAME=$1

USAGE="$0 CLUSTER_NAME"
DEFAULT_CLUSTER_NAME=kafka-connect
KAFKA_CONNECT_NETWORK_NAME=kafka-connect_net

# Check cluster name provided
if [ -z $CLUSTER_NAME ]
then
	echo No cluster name specified - assuming $DEFAULT_CLUSTER_NAME
	CLUSTER_NAME=$DEFAULT_CLUSTER_NAME
fi

# Check cluster exists
NODES=`aerolab cluster-list | awk '{print $1}' | grep -e aero-${CLUSTER_NAME}_[0-9]*$ | sort`

if [ -z "$NODES" ]
then
	echo No Aerospike cluster with name $CLUSTER_NAME found
	exit 1
fi


# Remove cluster nodes from network
for NODE in $NODES
do
	NETWORK_CONNECT_RESPONSE=`docker network disconnect "${KAFKA_CONNECT_NETWORK_NAME}" "${NODE}" 2>&1` 
	EXIT_CODE=$?
	if [ $EXIT_CODE == 0 ]
	then
		echo "Removed ${NODE} from network ${KAFKA_CONNECT_NETWORK_NAME}"
	elif [ $EXIT_CODE == 1 ]
	then
		echo "Node ${NODE} already removed from network ${KAFKA_CONNECT_NETWORK_NAME}"
	else
		echo "Following error after trying to remove ${NODE} from ${KAFKA_CONNECT_NETWORK_NAME}"
		echo
		echo $NETWORK_CONNECT_RESPONSE
	fi
	# Deliberate blank line
	echo
done

echo "Taking down Kafka/kafka-connect/zookeeper containers"
# Deliberate blank line
echo
DOCKER_COMPOSE_DOWN_OUTPUT=`docker-compose down 2>&1`

EXIT_CODE=$?
if [ $EXIT_CODE == 0 ]
then
	echo "Take down completed successfully"
	echo
else
	echo "A problem occurred when tearing down Kafka/kafka-connect/zookeeper"
	echo
	echo $DOCKER_COMPOSE_DOWN_OUTPUT
	echo
	exit 1
fi

