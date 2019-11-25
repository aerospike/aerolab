# Be more specific in docker network create

CLUSTER_NAME=$1

USAGE="$0 CLUSTER_NAME"
DEFAULT_CLUSTER_NAME=kafka-connect
KAFKA_CONNECT_IMAGE_NAME=aerospike/kafka-connect
KAFKA_CONNECT_NETWORK_NAME=kafka-connect_net
FEATURES_PATH=./cluster-setup/features.conf
KAFKA_CONNECT_BUILD_DIR=kafka-connect-build

CREDENTIALS_FILE=cluster-setup/credentials.conf
CITRUSLEAF_USER=citrusleaf

get_citrusleaf_password(){
	# Check credentials file exists
	if [ ! -e $CREDENTIALS_FILE ]
	then
		echo "Credentials file ${CREDENTIALS_FILE} not found - exiting"
		exit 1
	fi

	# Check we are using the citrusleaf user
	USER=$(grep User $CREDENTIALS_FILE | awk 'BEGIN{FS="="}{print $2}'| sed 's/"//g')

	if [ ! $USER == $CITRUSLEAF_USER ]
	then
		echo "Citrusleaf user ${CITRUSLEAF_USER} not used in $CREDENTIALS_FILE"
		exit 1
	fi

	CITRUSLEAF_PASSWORD=$(grep Pass $CREDENTIALS_FILE | awk 'BEGIN{FS="="}{print $2}'| sed 's/"//g')

	if [ -z $CITRUSLEAF_PASSWORD ]
	then
		echo "Citrusleaf password not found - exiting"
		exit 1
	fi	
}
# Test function to see if our docker kafka-connect docker image exists
kafka_connect_image_exists_output(){
	KAFKA_CONNECT_IMAGE_EXISTS_OUTPUT=`docker image list | grep $KAFKA_CONNECT_IMAGE_NAME | awk '{print $1}'`	
}

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

# Check whether KAFKA_CONNECT image exists
kafka_connect_image_exists_output
if [ $KAFKA_CONNECT_IMAGE_NAME == "$KAFKA_CONNECT_IMAGE_EXISTS_OUTPUT" ]
then
	echo KAFKA_CONNECT docker image found in registry
else
	get_citrusleaf_password
	echo "Building kafka-connect image"
	if [ ! -e $FEATURES_PATH ]
	then
		echo "Feature key file not found at $FEATURES_PATH - exiting"
	else
		cp $FEATURES_PATH $KAFKA_CONNECT_BUILD_DIR
	fi
	docker build --build-arg citrusleaf_pass=${CITRUSLEAF_PASSWORD} -t $KAFKA_CONNECT_IMAGE_NAME $KAFKA_CONNECT_BUILD_DIR
fi

# Check whether KAFKA_CONNECT image exists
kafka_connect_image_exists_output
if [ $KAFKA_CONNECT_IMAGE_NAME !=  "$KAFKA_CONNECT_IMAGE_EXISTS_OUTPUT" ]
then
	echo "kafka-connect image not found - exiting"
	exit 1
fi

# Set up network if it doesn't exist already
NETWORK_INFO=`docker network list | grep $KAFKA_CONNECT_NETWORK_NAME`

if [ -z "$NETWORK_INFO" ]
then
	NETWORK_ID=`docker network create --driver bridge $KAFKA_CONNECT_NETWORK_NAME`
	echo "Created docker network $KAFKA_CONNECT_NETWORK_NAME"
else
	echo "Network $KAFKA_CONNECT_NETWORK_NAME found - not creating"
fi

# Deliberate blank line
echo

# Add cluster nodes to cluster network
for NODE in $NODES
do
	NETWORK_CONNECT_RESPONSE=`docker network connect "${KAFKA_CONNECT_NETWORK_NAME}" "${NODE}" 2>&1` 
	EXIT_CODE=$?
	if [ $EXIT_CODE == 0 ]
	then
		echo "Added ${NODE} to network ${KAFKA_CONNECT_NETWORK_NAME}"
	elif [ $EXIT_CODE == 1 ]
	then
		echo "Node ${NODE} already added to network ${KAFKA_CONNECT_NETWORK_NAME}"
	else
		echo "Following error after trying to add ${NODE} to ${KAFKA_CONNECT_NETWORK_NAME}"
		echo
		echo $NETWORK_CONNECT_RESPONSE
	fi
	# Deliberate blank line
	echo
done

docker-compose up -d
echo "Kafka / kafka-connect & zookeeper initialized"
# docker-compose up --scale kafka=3 -d
# echo "Kafka scaled to 3 nodes"


#echo "Prometheus / Grafana containers created"
#echo
#echo "You should find your Grafana dashboards on localhost:3000"
#echo "and your Prometheus endpoint at localhost:9090"