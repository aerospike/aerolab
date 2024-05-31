set -e
. ./configure.sh
# create clients
if [ "${BACKEND}" == "aws" ]
then
  if [ ${#AWS_AVAILABILITY_ZONES[@]} -lt 1 ]
  then
    echo "AWS_AVAILABILITY_ZONES must have at least one AZ, for example: AWS_AVAILABILITY_ZONES=(us-east-1a us-east-1b)"
    exit 1
  fi
  echo "Creating clients, ${CLIENTS_PER_AZ} nodes per AZ, AZs=${AWS_AVAILABILITY_ZONES}"
else
  echo "Create cluster in docker, total nodes=$(( ${#AWS_AVAILABILITY_ZONES[@]} * ${CLIENTS_PER_AZ} ))"
fi

echo "Creating clients"
set -e
STAGE="create"
ZONELIST=${AWS_AVAILABILITY_ZONES}
[ "${BACKEND}" = "gcp" ] && ZONELIST=${GCP_AVAILABILITY_ZONES}
for i in ${ZONELIST[@]}
do
  aerolab client ${STAGE} tools -n ${CLIENT_NAME} -c ${CLIENTS_PER_AZ} -v ${VER} --instance-type ${CLIENT_AWS_INSTANCE} --ebs=${ROOT_VOL} --subnet-id=${i} --instance ${GCP_CLIENT_INSTANCE} --zone ${i} --disk=pd-ssd:${ROOT_VOL}
  STAGE="grow"
done

# copy astools
echo "Copy astools"
aerolab files upload -c -n ${CLIENT_NAME} astools.conf /etc/aerospike/astools.conf || exit 1

# configure asbench monitoring
aerolab client configure tools -l all -n ${CLIENT_NAME} --ams ${AMS_NAME}
