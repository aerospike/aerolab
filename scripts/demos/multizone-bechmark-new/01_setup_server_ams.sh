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
  echo "Creating cluster, ${NODES_PER_AZ} nodes per AZ, AZs=${AWS_AVAILABILITY_ZONES}"
else
  AWS_AVAILABILITY_ZONES=(docker)
  echo "Create cluster in docker, total nodes=$(( ${#AWS_AVAILABILITY_ZONES[@]} * ${NODES_PER_AZ} ))"
fi

sed "s/_NAMESPACE_/${NAMESPACE}/g" ${TEMPLATE} > aerospike.conf
STAGE="create"
for i in ${AWS_AVAILABILITY_ZONES[@]}
do
  aerolab cluster ${STAGE} -n ${CLUSTER_NAME} -c ${NODES_PER_AZ} -v ${VER} -o ${TEMPLATE} --instance-type ${CLUSTER_AWS_INSTANCE} --ebs=${AWS_EBS} --subnet-id=${i} --start=n
  STAGE="grow"
done
rm -f aerospike.conf

#### partition disks ####
echo "Partitioning disks"
if [ "${BACKEND}" == "aws" ]
then
  aerolab cluster partition create -n ${CLUSTER_NAME} --filter-type=nvme -p ${AWS_PARTITIONS}
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
echo "Copy astools"
aerolab files upload -n ${CLUSTER_NAME} astools.conf /etc/aerospike/astools.conf

#### apply roster ####
echo "SC-Roster"
RET=1
while [ ${RET} -ne 0 ]
do
  aerolab roster apply -m ${NAMESPACE} -n ${CLUSTER_NAME}
  RET=$?
  [ ${RET} -ne 0 ] && sleep 10
done

#### exporter ####
echo "Adding exporter"
aerolab cluster add exporter -n ${CLUSTER_NAME} -o ape.toml

#### deploy ams ####
echo "AMS"
aerolab client create ams -n ${AMS_NAME} -s ${CLUSTER_NAME} --instance-type ${AMS_AWS_INSTANCE} --ebs=40 --subnet-id=${AWS_AZ_1}
echo
aerolab client list |grep ${AMS_NAME}
