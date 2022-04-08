#!/bin/bash
version="v1.2"

source ./config

function getcontainerip() {
  ip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${1}_${2}_1 2>/dev/null)
  [ $? -ne 0 ] && ip=$(docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${1}-${2}-1 2>/dev/null)
  if [ -z ${ip} ]
  then
    return 1
  fi
  echo ${ip}
}

function getcontainername() {
  name=""
  docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${1}_${2}_1 1>/dev/null 2>&1
  if [ $? -eq 0 ]
  then
    name="${1}_${2}_1"
  fi
  docker inspect -f '{{range.NetworkSettings.Networks}}{{.IPAddress}}{{end}}' ${1}-${2}-1 1>/dev/null 2>&1
  if [ $? -eq 0 ]
  then
    name="${1}-${2}-1"
  fi
  if [ -z ${name} ]
  then
    return 1
  fi
  echo ${name}
}

LOC=$(pwd)
LDAPLOC=../aerolab-ldap
CLIENTLOC=../aerolab-pythonclient
GOCLIENTLOC=../aerolab-goclient
TEMPLATELOC=../../templates
function help() {
  if [ "${BUILD_PYTHON}" = "YES" ]
  then
    cd ${CLIENTLOC}
    CLIENT_NAME=$(getcontainername $(basename $(pwd)) ${SHORT_CLIENT_NAME}) || exit
  fi
  if [ "${BUILD_GO}" = "YES" ]
  then
    cd ${GOCLIENTLOC}
    GOCLIENT_NAME=$(getcontainername $(basename $(pwd)) ${SHORT_GOCLIENT_NAME}) || exit
  fi
  cd ${LOC}
  echo "Version : "${version}
  echo
  echo
  echo "Using asadm"
  echo "-----------"
  echo "asadm --auth=EXTERNAL_INSECURE -U badwan -P blastoff"
  echo "asadm --auth=EXTERNAL_INSECURE -U customuser -P blastoff"
  echo "asadm --auth=INTERNAL -U admin -P admin"
  echo "asadm --auth=EXTERNAL -U customuser -P blastoff --tls-enable --tls-cafile /etc/aerospike/rootCA.pem -p 4333 -t server1"
  echo
  echo "Using aql"
  echo "------------"
  echo "aql --auth=EXTERNAL -U customuser -P blastoff --tls-enable --tls-cafile /etc/aerospike/rootCA.pem -p 4333 -h server1"
  echo
  echo "Using standard ldapsearch"
  echo "-------------------------"
  echo "env LDAPTLS_CACERT=/etc/aerospike/rootCA.pem ldapsearch -H ldaps://ldap1:636 -b \"dc=aerospike,dc=com\" -D \"cn=admin,dc=aerospike,dc=com\" -w \"admin\" uid=badwan"
  echo
  echo "Attaching to aerospike & client nodes"
  echo "-------------------------------------"
  echo "aerolab node-attach -n ${CLUSTER_NAME} -l 1"
  if [ "${BUILD_PYTHON}" = "YES" ]
  then
    echo "docker exec -it ${CLIENT_NAME} /bin/bash"
  fi
  if [ "${BUILD_GO}" = "YES" ]
  then
    echo "docker exec -it ${GOCLIENT_NAME} /bin/bash"
  fi
  echo
  echo
  echo "To display this help again, run: ${0} help"
  echo
  echo
  exit
}

if [ "${1}" != "" ]
then
  cd ${LDAPLOC}
  LDAP_NAME=$(getcontainername $(basename $(pwd)) ${SHORT_LDAP_NAME})
  if [ $? -ne 0 ]
  then
    echo "Version : "${version}
    echo
    echo "E: Not built yet; deploy it first by running without any parameters"
    exit
  fi
  help
fi
cd ${LOC}
${LOC}/destroy-env.sh || exit

echo "Configuring LDAP to : "${SHORT_LDAP_NAME}
cp ${TEMPLATELOC}/${TEMPLATE} .
sed -i.bak -e "s/LDAPIP/${SHORT_LDAP_NAME}/g" ${TEMPLATE}

echo
echo "Create New ldap server"
cd ${LDAPLOC}
./runme.sh run
echo
echo "Get LDAPIP"
LDAPIP=$(getcontainerip $(basename $(pwd)) ${SHORT_LDAP_NAME}) || exit
cd ${LOC}

echo
echo "Build Aerospike"
aerolab make-cluster -n ${CLUSTER_NAME} -c ${NODES} ${VERSION} -o ${LOC}/${TEMPLATE} ${FEATURES} ${NETWORKMODE} -s n
aerolab upload -n ${CLUSTER_NAME} -i ${LDAPLOC}/certs/local/rootCA.pem -o /etc/aerospike/rootCA.pem
aerolab upload -n ${CLUSTER_NAME} -i ${LDAPLOC}/certs/output/server1.pem -o /etc/aerospike/server1.pem
aerolab upload -n ${CLUSTER_NAME} -i ${LDAPLOC}/certs/output/server1.key -o /etc/aerospike/server1.key
for NODE in $(seq 1 ${NODES})
do
  NODEIP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' aero-${CLUSTER_NAME}_${NODE}) || exit
  echo "Fix Hosts File for :"${NODEIP}
  aerolab node-attach -n ${CLUSTER_NAME} -l ${NODE} -- sh -c "echo ${LDAPIP} ${SHORT_LDAP_NAME} >> /etc/hosts"
  aerolab node-attach -n ${CLUSTER_NAME} -l ${NODE} -- sh -c "sed -i.bak -e 's/^${NODEIP}/${NODEIP} server1/g' /etc/hosts"
  echo "Install ldapsearch"
  aerolab node-attach -n ${CLUSTER_NAME} -l ${NODE} -- sh -c "apt-get install ldap-client >/dev/null 2>/dev/null"
  
done
aerolab start-aerospike -n ${CLUSTER_NAME}

echo "Get CLUSTERIP"
CLUSTERIP=$(docker inspect -f '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}' aero-${CLUSTER_NAME}_1) || exit
echo

if [ "${BUILD_PYTHON}" = "YES" ]
then
  echo "Configuring Python to : "${CLUSTERIP}
  cd ${CLIENTLOC}
  ./runme.sh run
  CLIENTIP=$(getcontainerip $(basename $(pwd)) ${SHORT_CLIENT_NAME}) || exit
  CLIENT_NAME=$(getcontainername $(basename $(pwd)) ${SHORT_CLIENT_NAME}) || exit

  cd clients
  for i in *.py
  do
    sed -i.bak -e "s/CLUSTERIP/${CLUSTERIP}/g" ${i}
    chmod 755 ${i}
  done

  cd ${LOC}
fi

if [ "${BUILD_GO}" = "YES" ]
then
  echo "Configuring GO to : "${CLUSTERIP}

  cd ${GOCLIENTLOC}
  ./runme.sh run
  GOCLIENTIP=$(getcontainerip $(basename $(pwd)) ${SHORT_GOCLIENT_NAME}) || exit
  GOCLIENT_NAME=$(getcontainername $(basename $(pwd)) ${SHORT_GOCLIENT_NAME}) || exit

  cd go/src/aerospike
  for i in *.go
  do
    sed -i.bak -e "s/CLUSTERIP/${CLUSTERIP}/g" ${i}
  done

  cd ${LOC}
fi
help
