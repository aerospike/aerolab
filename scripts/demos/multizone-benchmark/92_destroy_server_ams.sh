. ./configure.sh
aerolab client destroy -f -n ${AMS_NAME}
aerolab cluster destroy -f -n ${CLUSTER_NAME}
echo "Don't forget to run destroy_clients.sh too"
