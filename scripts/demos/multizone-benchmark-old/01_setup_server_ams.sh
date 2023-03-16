. ./configure.sh

# prepare config files
TNF=template.conf
[ "${PROVISION}" != "" ] && TNF=template_nvme.conf
sed 's/_RACKID_/1/g' ${TNF} |sed "s/_NAMESPACE_/${NAMESPACE}/g" > rack1.conf
sed 's/_RACKID_/2/g' ${TNF} |sed "s/_NAMESPACE_/${NAMESPACE}/g" > rack2.conf
sed 's/_RACKID_/3/g' ${TNF} |sed "s/_NAMESPACE_/${NAMESPACE}/g" > rack3.conf

# create cluster
echo "Creating cluster"
aerolab cluster create -n ${NAME} -c 2 -v ${VER} -o rack1.conf --instance-type ${AWS_INSTANCE} --ebs=40 --subnet-id=${AWS_AZ_1} || exit 1
aerolab cluster grow -n ${NAME} -c 2 -v ${VER} -o rack2.conf --instance-type ${AWS_INSTANCE} --ebs=40 --subnet-id=${AWS_AZ_2} || exit 1
aerolab cluster grow -n ${NAME} -c 2 -v ${VER} -o rack3.conf --instance-type ${AWS_INSTANCE} --ebs=40 --subnet-id=${AWS_AZ_3} || exit 1

if [ "${PROVISION}" != "" ]
then
  for i in $(echo ${PROVISION})
  do
    cat <<EOF > partitioner.sh
      blkdiscard $i
      parted -s $i "mktable gpt"
      parted -a optimal -s $i "mkpart primary 0% 20%"
      parted -a optimal -s $i "mkpart primary 20% 40%"
      parted -a optimal -s $i "mkpart primary 40% 60%"
      parted -a optimal -s $i "mkpart primary 60% 80%"
      sleep 10
      blkdiscard -z --length 8MiB ${i}p1
      blkdiscard -z --length 8MiB ${i}p2
      blkdiscard -z --length 8MiB ${i}p3
      blkdiscard -z --length 8MiB ${i}p4
EOF
    aerolab files upload -n ${NAME} partitioner.sh /opt/partitioner.sh || exit 1
    rm -f partitioner.sh
    aerolab attach shell -n ${NAME} -l all -- bash /opt/partitioner.sh || exit 1
    aerolab aerospike stop -n ${NAME} -l all
    sleep 5
    aerolab aerospike start -n ${NAME} -l all
  done
fi

# remove config files
rm -f rack1.conf rack2.conf rack3.conf

# let the cluster do it's thang
echo "Wait"
sleep 15

# setup security
echo "Security"
aerolab attach asadm -n ${NAME} -- -U admin -P admin -e "enable; manage acl create role superuser priv read-write-udf" || exit 1
aerolab attach asadm -n ${NAME} -- -U admin -P admin -e "enable; manage acl grant role superuser priv sys-admin" || exit 1
aerolab attach asadm -n ${NAME} -- -U admin -P admin -e "enable; manage acl grant role superuser priv user-admin" || exit 1
aerolab attach asadm -n ${NAME} -- -U admin -P admin -e "enable; manage acl create user superman password krypton roles superuser" || exit 1

# copy astools
echo "Copy astools"
aerolab files upload -n ${NAME} astools.conf /etc/aerospike/astools.conf || exit 1

# apply roster
echo "SC-Roster"
RET=1
while [ ${RET} -ne 0 ]
do
  aerolab roster apply -m ${NAMESPACE} -n ${NAME}
  RET=$?
  [ ${RET} -ne 0 ] && sleep 10
done

# exporter
echo "Adding exporter"
aerolab cluster add exporter -n ${NAME} -o ape.toml || exit 1

# deploy ams
echo "AMS"
aerolab client create ams -n ${AMS_NAME} -s ${NAME} --instance-type ${AWS_CLIENT_INSTANCE} --ebs=40 --subnet-id=${AWS_AZ_1} || exit 1
echo
aerolab client list |grep ${AMS_NAME}
