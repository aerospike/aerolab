set -e
. ./configure.sh

#### create cluster ####
if [ "${BACKEND}" == "aws" ]
then
  if [ ${#AWS_AVAILABILITY_ZONES[@]} -lt 1 ]
  then
    echo "AWS_AVAILABILITY_ZONES must have at least one AZ, for example: AWS_AVAILABILITY_ZONES=(us-east-1a us-east-1b)"
    exit 1
  fi
  echo "Creating cluster, ${NODES_PER_AZ} nodes per AZ, AZs="${AWS_AVAILABILITY_ZONES[@]}
elif [ "${BACKEND}" == "gcp" ]
  if [ ${#GCP_AVAILABILITY_ZONES[@]} -lt 1 ]
  then
    echo "GCP_AVAILABILITY_ZONES must have at least one AZ, for example: GCP_AVAILABILITY_ZONES=(us-central1-a us-central1-c)"
    exit 1
  fi
  echo "Creating cluster, ${NODES_PER_AZ} nodes per AZ, AZs="${GCP_AVAILABILITY_ZONES[@]}
else
  echo "Create cluster in docker, total nodes=$(( ${#AWS_AVAILABILITY_ZONES[@]} * ${NODES_PER_AZ} ))"
fi

TEMPLATE=template.conf
[ $ENABLE_STRONG_CONSISTENCY -eq 1 -a $ENABLE_SECURITY -eq 0 ] && TEMPLATE=template-sc.conf
[ $ENABLE_STRONG_CONSISTENCY -eq 0 -a $ENABLE_SECURITY -eq 1 ] && TEMPLATE=template-security.conf
[ $ENABLE_STRONG_CONSISTENCY -eq 1 -a $ENABLE_SECURITY -eq 1 ] && TEMPLATE=template-sc-security.conf

sed "s/_NAMESPACE_/${NAMESPACE}/g" ${TEMPLATE} > aerospike.conf
STAGE="create"
START_NODE=0
END_NODE=0
RACK_NO=0
ZONELIST=${AWS_AVAILABILITY_ZONES}
[ "${BACKEND}" = "gcp" ] && ZONELIST=${GCP_AVAILABILITY_ZONES}
for i in ${ZONELIST[@]}
do
  START_NODE=$(( ${END_NODE} + 1 ))
  END_NODE=$(( ${START_NODE} + ${NODES_PER_AZ} - 1 ))
  RACK_NO=$(( ${RACK_NO} + 1 ))
  nodes=""
  for j in $(seq ${START_NODE} ${END_NODE})
  do
    [ "${nodes}" = "" ] && nodes=${j} || nodes="${nodes},${j}"
  done
  aerolab cluster ${STAGE} -n ${CLUSTER_NAME} -c ${NODES_PER_AZ} -v ${VER} -o aerospike.conf --instance-type ${CLUSTER_AWS_INSTANCE} --ebs=${ROOT_VOL} --subnet-id=${i} --start=n --instance ${CLUSTER_GCP_INSTANCE} --disk=pd-ssd:${ROOT_VOL} --disk=local-ssd@${GCP_LOCAL_SSDS} --zone ${i}
  aerolab conf rackid -n ${CLUSTER_NAME} -l ${nodes} -i ${RACK_NO} -m ${NAMESPACE} -r -e
  STAGE="grow"
done
rm -f aerospike.conf

#### partition disks ####
echo "Partitioning disks"
if [ "${BACKEND}" != "docker" ]
then
  aerolab cluster partition create -n ${CLUSTER_NAME} --filter-type=nvme -p ${AWS_GCP_PARTITIONS}
  aerolab cluster partition conf -n ${CLUSTER_NAME} --namespace=${NAMESPACE} --filter-type=nvme --configure=device
fi

#### start cluster ####
echo "Starting cluster"
aerolab aerospike start -n ${CLUSTER_NAME} -l all

#### let the cluster do it's thang ####
echo "Wait"
sleep 15

#### setup security ####
echo "Security"
aerolab attach asadm -n ${CLUSTER_NAME} -- -U admin -P admin -e "enable; manage acl create role superuser priv read-write-udf"
aerolab attach asadm -n ${CLUSTER_NAME} -- -U admin -P admin -e "enable; manage acl grant role superuser priv sys-admin"
aerolab attach asadm -n ${CLUSTER_NAME} -- -U admin -P admin -e "enable; manage acl grant role superuser priv user-admin"
aerolab attach asadm -n ${CLUSTER_NAME} -- -U admin -P admin -e "enable; manage acl create user superman password krypton roles superuser"

#### copy astools ####
if [ $ENABLE_SECURITY -eq 1 ]
then
  echo "Copy astools"
  aerolab files upload -n ${CLUSTER_NAME} astools.conf /etc/aerospike/astools.conf
fi

#### apply roster
if [ $ENABLE_STRONG_CONSISTENCY -eq 1 ]
then
  echo "SC-Roster"
  RET=1
  while [ ${RET} -ne 0 ]
  do
    aerolab roster apply -m ${NAMESPACE} -n ${CLUSTER_NAME}
    RET=$?
    [ ${RET} -ne 0 ] && sleep 10
  done
fi

#### exporter ####
echo "Adding exporter"
if [ $ENABLE_SECURITY -eq 1 ]
then
  aerolab cluster add exporter -n ${CLUSTER_NAME} -o ape.toml
else
  aerolab cluster add exporter -n ${CLUSTER_NAME}
fi

#### deploy ams ####
echo "AMS"
aerolab client create ams -n ${AMS_NAME} -s ${CLUSTER_NAME} --instance-type ${AMS_AWS_INSTANCE} --ebs=${ROOT_VOL} --subnet-id=${ZONELIST[0]} --instance ${AMS_GCP_INSTANCE} --disk=pd-ssd:${ROOT_VOL} --zone ${ZONELIST[0]}
echo
aerolab client list |grep ${AMS_NAME}