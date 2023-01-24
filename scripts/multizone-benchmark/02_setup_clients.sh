. configure.sh

# create clients
echo "Creating clients"
aerolab client create tools -n ${CLIENT_NAME} -v ${VER} -c ${CLIENTS} --instance-type ${AWS_CLIENT_INSTANCE} --ebs=40 --subnet-id=${AWS_AZ_1} || exit 1

# copy astools
echo "Copy astools"
aerolab files upload -c -n ${CLIENT_NAME} astools.conf /etc/aerospike/astools.conf || exit 1

# configure asbench monitoring
aerolab client configure tools -l all -n ${CLIENT_NAME} --ams ${AMS_NAME}