#!/bin/bash
# Location of aerolab binary
AEROLABDEFAULT="/Users/ajohnson/Aerolab/aerolab-osx-aio"
if ! AEROLAB="$(type -p aerolab)" || [[ -z $AEROLAB ]]
then
  AEROLAB=${AEROLABDEFAULT}
fi

if [ ! -e "${AEROLAB}" ]
then
  echo "You need to configure your aerolab binary location"
  exit 0
else
  echo "Found aerolab here : ${AEROLAB}"
fi

# Name of your cluster
LAB_NAME="test_dsc"

# Number of nodes to build with 2-9
NODES=5

# Namespace to set up
NAMESPACE="bar"

# Default Replication Factor
REPLICATION=2

# Built with STRONGCONSISTENCY
STRONGCONSISTENCY=true

# If you have the name of a binary here it will copy it to the cluster for testing specific versions
#ASD="asd-5.7.0.7-2"

function usage {
  echo
  echo $(basename $0)
  echo "Usage :-"
  echo
  echo " -c|--nodes <node_count> default=${NODES} : Range 1-9"
  echo " -r|--replication <replication_factor> default=${REPLICATION} : Range 1-${NODES}"
  echo " -n|--namespace <namespace> default=${NAMESPACE}"
  echo " -a|--asdver <new_binary> default=${ASD}"
  echo " -l|--labname <name_of_cluster> default=${LAB_NAME}"
  echo " -s|--sc <true/false> default=${STRONGCONSISTENCY}"
  echo " -?|-h|--help This Messaage"
  echo
  echo
  exit
}

while [ $# -gt 0 ]
do
  opt=$1
  case ${opt} in
    --nodes)        NODES=$2;shift;;
    -c)             NODES=$2;shift;;
    --replication)  REPLICATION=$2;shift;;
    -r)             REPLICATION=$2;shift;;
    --namespace)    NAMESPACE=$2;shift;;
    -n)             NAMESPACE=$2;shift;;
    --asdver)       ASD=$2;shift;;
    -a)             ASD=$2;shift;;
    --labname)      LAB_NAME=$2;shift;;
    -l)             LAB_NAME=$2;shift;;
    --sc)           STRONGCONSISTENCY=$2;shift;;
    -s)             STRONGCONSISTENCY=$2;shift;;
    -h)             usage;;
    --help)         usage;;
    -?)             usage;;
    *)              usage;;
  esac
  shift
done

if [ ! "${STRONGCONSISTENCY}" = "true" -a ! "${STRONGCONSISTENCY}" = "false" ]
then
  echo "Invalid Strong Consistency"
  echo
  usage
fi
if [ ! -z "${ASD}" -a ! -f "${ASD}" ]
then
  echo "ASD Binary not found"
  echo
  usage
fi
RE='^[0-9]+$'
if ! [[ "${NODES}" =~ $RE ]]
then
  echo "Nodes not numeric"
  echo
  usage
fi
if [ "${NODES}" -gt 9 -o "${NODES}" -lt 1 ]
then
  echo "Nodes value incorrect"
  echo
  usage
fi

if ! [[ "${REPLICATION}" =~ $RE ]]
then
  echo "Replication Factor not numeric"
  echo
  usage
fi

if [ ${REPLICATION} -gt ${NODES} ]
then
  echo "Replication/Node number mis-configuration"
  echo
  usage
fi

echo
echo "##############################################################"
echo "## *** WARNING *** Building a cluster will first destroy it ##"
echo "##############################################################"
if [ ! -z ${ASD} ]
then
  echo
  echo "We will upgrade to this binary : ${ASD}"
  echo
fi
echo
echo "I am going to build the following cluster :-"
echo "Name      :${LAB_NAME}"
echo "Nodes     :${NODES}"
echo "RF        :${REPLICATION}"
echo "Namespace :${NAMESPACE}"
echo "SC        :${STRONGCONSISTENCY}"
echo
echo -ne "Continue (Y/N) ?"
read yn
if [  "${yn}" = "Y" -o  "${yn}" = "y" ]
then
  echo "Building..."
else
  echo "Aborting"
  exit 0
fi
echo

echo "Creating Config"

# Create a config to put into the cluster
cat <<'EOF' >aerospike.conf
# Aerospike database configuration file for use with systemd.
service {
	paxos-single-replica-limit 1 # Number of nodes where the replica count is automatically reduced to 1.
	proto-fd-max 15000

        node-id NODEID
}
logging {
	file /var/log/aerospike.log {
		context any info
	}
}
network {
	service {
		address any
		port 3000
	}
	heartbeat {
mode mesh
port 3002
		# To use unicast-mesh heartbeats, remove the 3 lines above, and see
		# aerospike_mesh.conf for alternative.
		interval 150
		timeout 10
	}
	fabric {
		port 3001
	}
	info {
		port 3003
	}
}
namespace NAMESPACE {
	replication-factor REPLICATION
	memory-size 2G
        strong-consistency STRONGCONSISTENCY
	storage-engine device {
		file /opt/aerospike/data/NAMESPACE.dat
		filesize 2G
		data-in-memory true # Store data in memory in addition to file.
	}
}
EOF

echo "Stop and destroy the cluster if it already exists"
${AEROLAB} stop-aerospike -n ${LAB_NAME} 2>/dev/null
${AEROLAB} cluster-stop -n ${LAB_NAME} 2>/dev/null
${AEROLAB} cluster-destroy -n ${LAB_NAME} 2>/dev/null

echo "Make new cluster : "${LAB_NAME}
${AEROLAB} make-cluster -n ${LAB_NAME} -c ${NODES} -s n 2>/dev/null
sleep 2

# Configure the nodes
for i in `seq 1 ${NODES}`
do
   echo "Configuring Node :"${i}
   ${AEROLAB} upload -n ${LAB_NAME} -l ${i} -i aerospike.conf -o /etc/aerospike/aerospike.conf 2>/dev/null
   if [ ! -z "${ASD}" ]
   then
     echo "Upgrading Binary"
     ${AEROLAB} upload -n ${LAB_NAME} -l ${i} -i ${ASD} -o /usr/bin/asd 2>/dev/null
     ${AEROLAB} node-attach -n ${LAB_NAME} -l ${i} -- chmod 755 /usr/bin/asd
   fi
   ${AEROLAB} node-attach -n ${LAB_NAME} -l ${i} -- sed -i -e "s/NODEID/a${i}/g" -e "s/REPLICATION/${REPLICATION}/g" -e "s/NAMESPACE/${NAMESPACE}/g" -e "s/STRONGCONSISTENCY/${STRONGCONSISTENCY}/g" /etc/aerospike/aerospike.conf
done

echo "Fix networking & Start Aerospike"
${AEROLAB} conf-fix-mesh -n ${LAB_NAME}
${AEROLAB} start-aerospike -n ${LAB_NAME}
sleep 5

# Configure observed node list for SC and recluster
if [ "${STRONGCONSISTENCY}" = "true" ]
then
  echo "Getting observed list"
  sleep 5
  OBSERVED=`${AEROLAB} asadm -n ${LAB_NAME} -l 1 -- -e "enable;asinfo -v 'roster:namespace=${NAMESPACE}'" | grep roster= | head -n1 | cut -f4- -d "=" | egrep -o '[A0-9,]+'`
  echo "Configure Observered list :"${OBSERVED}": and Recluster"
  
  ${AEROLAB} asadm -n ${LAB_NAME} -l 1 -- "-e enable;asinfo -v 'roster-set:namespace=${NAMESPACE};nodes=${OBSERVED}'"
  ${AEROLAB} asadm -n ${LAB_NAME} -l 1 -- "-e enable;asinfo -v 'recluster:'"
fi

echo
echo "Ready to insert data : aerolab insert-data -n ${LAB_NAME} -m ${NAMESPACE} -z 10000"
# Done
