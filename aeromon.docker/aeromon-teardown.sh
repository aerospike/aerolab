CLUSTER_NAME=$1

USAGE="$0 CLUSTER_NAME"
AEROMON_PROBE_SUFFIX=aeromon-probe

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

echo "Taking down Prometheus / Grafana containers"
# Deliberate blank line
echo
DOCKER_COMPOSE_DOWN_OUTPUT=`docker-compose down 2>&1`

EXIT_CODE=$?
if [ $EXIT_CODE == 0 ]
then
	echo "Take down completed successfully"
	echo
else
	echo "A problem occurred when tearing down Prometheus/Grafana"
	echo
	echo $DOCKER_COMPOSE_DOWN_OUTPUT
	echo
	exit 1
fi

# Remove aeromon probes
for NODE in $NODES
do
	AEROMON_PROBE_NAME=${NODE}-${AEROMON_PROBE_SUFFIX}
	echo "Stopping aeromon probe for cluster node ${AEROMON_PROBE_NAME}"
	echo
	STOP_AEROMON_RESPONSE=`docker container stop ${AEROMON_PROBE_NAME} 2>&1`
	EXIT_CODE=$?	
	if [ $EXIT_CODE == 0 ]
	then
		echo "Aeromon probe ${AEROMON_PROBE_NAME} stopped"
		echo
	else
		echo "An error occured"
		echo
		echo $STOP_AEROMON_RESPONSE
		echo		
	fi	
	REMOVE_AEROMON_RESPONSE=`docker container rm ${AEROMON_PROBE_NAME} 2>&1`
	EXIT_CODE=$?	
	if [ $EXIT_CODE == 0 ]
	then
		echo "Aeromon probe ${AEROMON_PROBE_NAME} removed"
		echo
	else
		echo "An error occured"
		echo
		echo $REMOVE_AEROMON_RESPONSE
		echo		
	fi	

done


