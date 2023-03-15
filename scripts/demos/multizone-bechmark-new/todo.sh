# names
CLUSTER=NAME="robert"
AMS_NAME="glonek"

# instances
CLUSTER_AWS_INSTANCE="r5ad.4xlarge"
AMS_AWS_INSTANCE="t3a.medium"

# namespace name
NAMESPACE="test"

# if backend is 'docker', total nodes is NODES_PER_AZ
BACKEND="aws"

# list of AWS AZs to deploy in
AWS_AVAILABILITY_ZONES=(us-east-1a us-east-1b)

# number of server nodes per AZ
NODES_PER_AZ=2

# size of the root volume
AWS_EBS=40

# partitions to create per NVMe if on AWS, split as percentages
AWS_PARTITIONS=25,25,25,25

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

set -e
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
aerolab attach asadm -n ${CLUSTER_NAME} -- -U admin -P admin -e "enable; manage acl create role superuser priv read-write-udf" || exit 1
aerolab attach asadm -n ${CLUSTER_NAME} -- -U admin -P admin -e "enable; manage acl grant role superuser priv sys-admin" || exit 1
aerolab attach asadm -n ${CLUSTER_NAME} -- -U admin -P admin -e "enable; manage acl grant role superuser priv user-admin" || exit 1
aerolab attach asadm -n ${CLUSTER_NAME} -- -U admin -P admin -e "enable; manage acl create user superman password krypton roles superuser" || exit 1

#### copy astools ####
echo "Copy astools"
aerolab files upload -n ${CLUSTER_NAME} astools.conf /etc/aerospike/astools.conf || exit 1

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
aerolab cluster add exporter -n ${CLUSTER_NAME} -o ape.toml || exit 1

#### deploy ams ####
echo "AMS"
aerolab client create ams -n ${AMS_NAME} -s ${CLUSTER_NAME} --instance-type ${AMS_AWS_INSTANCE} --ebs=40 --subnet-id=${AWS_AZ_1} || exit 1
echo
aerolab client list |grep ${AMS_NAME}

#### setup clients ####
echo "Creating clients"
TODO

#### run insert load ####
echo "Running insert load"
TODO

#### run RU load ####
echo "Running read-update load"
TODO

#### destroy ####
echo "Destroying everything"
TODO
