#!/bin/bash

source ./config

LOC=$(pwd)
LDAPLOC=../aerolab-ldap
CLIENTLOC=../aerolab-pythonclient
GOCLIENTLOC=../aerolab-goclient
TEMPLATELOC=../../templates

echo "Remove Old Containers"
aerolab cluster-stop -n ${CLUSTER_NAME} 1>/dev/null 2>&1
aerolab cluster-destroy -n ${CLUSTER_NAME} 1>/dev/null 2>&1

cd ${LDAPLOC}
./runme.sh destroy 1>/dev/null 2>&1
cd ${LOC}

cd ${CLIENTLOC}
./runme.sh destroy 1>/dev/null 2>&1
cd ${LOC}

cd ${GOCLIENTLOC}
./runme.sh destroy 1>/dev/null 2>&1
cd ${LOC}
