# Be more specific in docker network create

CLUSTER_NAME=$1

USAGE="$0 CLUSTER_NAME"
AEROMON_IMAGE_NAME=aerospike/aeromon
AEROMON_PROBE_SUFFIX=aeromon-probe

# Test function to see if our docker aeromon docker image exists
aeromon_image_exists_output(){
	AEROMON_IMAGE_EXISTS_OUTPUT=`docker image list | grep $AEROMON_IMAGE_NAME | awk '{print $1}'`	
}

# Check cluster name provided
if [ -z $CLUSTER_NAME ]
then
	echo usage : $USAGE
	exit 1
fi

# Check cluster exists
NODES=`aerolab cluster-list | awk '{print $1}' | grep -e aero-${CLUSTER_NAME}_[0-9]*$ | sort`

if [ -z "$NODES" ]
then
	echo No Aerospike cluster with name $CLUSTER_NAME found
	exit 1
fi

# Check whether aeromon image exists
aeromon_image_exists_output
if [ $AEROMON_IMAGE_NAME == "$AEROMON_IMAGE_EXISTS_OUTPUT" ]
then
	echo Aeromon docker image found in registry
else
	echo "Building aeromon image"
	source ./ssh-check.sh
	./aeromon-build.sh
fi

# Check whether aeromon image exists
aeromon_image_exists_output
if [ $AEROMON_IMAGE_NAME !=  "$AEROMON_IMAGE_EXISTS_OUTPUT" ]
then
	echo "Aeromon image not found - exiting"
	exit 1
fi

# Set up network if it doesn't exist already
NETWORK_NAME=aero-${CLUSTER_NAME}
NETWORK_INFO=`docker network list | grep $NETWORK_NAME`

if [ -z "$NETWORK_INFO" ]
then
	NETWORK_ID=`docker network create --driver bridge $NETWORK_NAME`
	echo "Created docker network $NETWORK_NAME"
else
	echo "Network $NETWORK_NAME found - not creating"
fi

# Deliberate blank line
echo

# Add cluster nodes to cluster network
for NODE in $NODES
do
	NETWORK_CONNECT_RESPONSE=`docker network connect "${NETWORK_NAME}" "${NODE}" 2>&1` 
	EXIT_CODE=$?
	if [ $EXIT_CODE == 0 ]
	then
		echo "Added ${NODE} to network ${NETWORK_NAME}"
	elif [ $EXIT_CODE == 1 ]
	then
		echo "Node ${NODE} already added to network ${NETWORK_NAME}"
	else
		echo "Following error after trying to add ${NODE} to ${NETWORK_NAME}"
		echo
		echo $NETWORK_CONNECT_RESPONSE
	fi
	# Deliberate blank line
	echo
done

# Create aeromon probe - one per cluster host
for NODE in $NODES
do
	echo "Creating aeromon probe for cluster node ${NODE}"
	echo
	AEROMON_PROBE_NAME=${NODE}-${AEROMON_PROBE_SUFFIX}
	RUN_AEROMON_RESPONSE=`docker run -d -e AEROSPIKE_HOST=${NODE} --name ${AEROMON_PROBE_NAME} --network ${NETWORK_NAME} ${AEROMON_IMAGE_NAME} 2>&1`
	EXIT_CODE=$?	
	if [ $EXIT_CODE == 0 ]
	then
		echo "Aeromon probe ${AEROMON_PROBE_NAME} created for cluster node ${NODE}"
	elif [ $EXIT_CODE == 125 ]
	then
		echo "Existing aeromon probe ${AEROMON_PROBE_NAME} found"
	else
		echo "An error occured"
		echo
		echo $RUN_AEROMON_RESPONSE
	fi	
	# Deliberate blank line
	echo
done

# Start Prometheus / Grafana in the aerospike network
cat templates/docker-compose.yml | sed "s/<NETWORK_NAME>/${NETWORK_NAME}/" > docker-compose.yml
echo "Prometheus/Grafana configured to start in ${NETWORK_NAME} network"

for NODE in $NODES
do
	AEROMON_PROBE_NAME=${NODE}-${AEROMON_PROBE_SUFFIX}
	TARGETS="${TARGETS},'${AEROMON_PROBE_NAME}:9145'"
done
# Configure targets
TARGETS=`echo $TARGETS | sed 's/^,//'`
cat templates/prometheus.yml | sed "s/<TARGETS>/${TARGETS}/" > docker/prometheus/prometheus.yml
echo "Prometheus targets set up as ${TARGETS}"
docker-compose up -d
echo "Prometheus / Grafana containers created"
echo
echo "You should find your Grafana dashboards on localhost:3000"
echo "and your Prometheus endpoint at localhost:9090"