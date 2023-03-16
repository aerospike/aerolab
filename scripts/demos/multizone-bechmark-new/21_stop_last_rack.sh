set -e
. ./configure.sh

END_NODE=$(( ${#AWS_AVAILABILITY_ZONES[@]} * ${NODES_PER_AZ} ))
START_NODE=$(( ${END_NODE} - ${NODES_PER_AZ} + 1 ))

nodes=""
for i in $(seq ${START_NODE} ${END_NODE})
do
  [ "${nodes}" = "" ] && nodes=${i} || nodes="${nodes},${i}"
done
aerolab aerospike stop -n ${CLUSTER_NAME} -l ${nodes}
