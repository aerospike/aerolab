#!/bin/bash

source client-tests.sh
source cluster-tests.sh

aerolab="./aerolab-macos"
export AEROLAB_CONFIG_FILE=test.conf
cluster="testcluster"
logDir="./log"
featureFile="/Users/rglonek/aerolab/templates/features.conf"
backend="docker"
extra=""
[ "${backend}" = "aws" ] && extra="${extra} -I t3a.medium -E 20 -S sg-03430d698bffb44a3 -U subnet-06cc8a834647c4cc3" #-L

logsExtra=""
[ "${backend}" = "aws" ] && logsExtra="-j"

cat <<'EOF' > strong.conf
service {
        proto-fd-max 15000
}
logging {
        console {
                context any info
        }
}
network {
        service {
                address any
                port 3000
        }
        heartbeat {
                mode multicast
                multicast-group 239.1.99.222
                port 9918
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
namespace bar {
        replication-factor 2
        memory-size 4G
        default-ttl 0
        strong-consistency true
        storage-engine device {
                file /opt/aerospike/data/bar.dat
                filesize 1G
                data-in-memory false
        }
}
EOF
./build.sh || exit 1
rm -rf ${logDir}
mkdir -p ${logDir}
funcno=0

if [ "${1}" == "color" ]
then
    green=$(tput setaf 2)
    blue=$(tput setaf 4)
    red=$(tput setaf 1)
    normal=$(tput sgr0)
fi

function handle() {
    printf "%s %.30s " "${blue}$(date)${normal}" "${1} .................................."
    funcno=$((funcno+1))
    logFile=${logDir}/${funcno}-${1}.log
    ${1} >${logFile} 2>&1
    if [ $? -ne 0 ]
    then
        echo "${red}FAIL${normal}"
        exit 1
    else
        echo "${green}OK${normal}"
    fi
}

function end() {
    return 0
}

function setup() {
    rm -f ${AEROLAB_CONFIG_FILE}
    ${aerolab} config backend -t ${backend} -r us-east-1 || return 1
    ${aerolab} config backend |grep "config.backend.type=${backend}" || return 2
    ${aerolab} config defaults -k '*FeaturesFilePath' -v ${featureFile} || return 3
    ${aerolab} config defaults -k '*.HeartbeatMode' -v mesh || return 4
    cleanup || return 5
}

function pause() {
    sleep 5
}

function cleanup() {
    ${aerolab} cluster destroy -f -n test
    ${aerolab} cluster destroy -f -n tests
    ${aerolab} cluster destroy -f -n testd1
    ${aerolab} cluster destroy -f -n testd2
    check_template_destroy || return 4
    rm -f aerospike-server-enterprise-*.tgz
    rm -rf CA
}

function check_version() {
    ${aerolab} version
    return $?
}

function clusterchecks() {
    handle setup
    handle check_version
    handle check_installer_listversions
    handle check_installer_download
    handle check_cluster_create
    handle check_cluster_list
    handle check_cluster_stop
    handle check_cluster_start
    handle check_cluster_grow
    handle check_conf_fixmesh
    handle check_aerospike_stop
    handle check_aerospike_start
    handle check_aerospike_restart
    handle check_aerospike_upgrade
    handle check_files_edit
    handle check_files_sync
    handle check_files_upload
    handle check_files_download
    handle check_logs_show
    handle check_logs_get
    handle pause
    handle check_attach_shell
    handle check_attach_aql
    handle check_attach_asadm
    handle check_attach_asinfo
    handle check_roster_apply
    handle check_roster_show
    handle check_data_insert
    handle check_attach_asadm
    handle check_data_delete
    handle check_attach_asadm
    handle check_net_block
    handle check_net_list
    handle check_net_unblock
    handle check_net_lossdelay
    handle check_cluster_destroy
    handle check_template_destroy
    handle check_template_create
    handle check_xdr_create
    handle check_xdr_details
    handle check_xdr_connect
    handle check_xdr_details
    handle check_tls_generate
    handle check_tls_copy
    [ "${backend}" = "aws" ] && handle check_xdr_crossregion
    handle cleanup
}

function clientchecks() {
    handle setup
    handle check_version
    handle check_client_create_add
    handle check_client_list
    handle check_client_attach
    handle check_client_stop
    handle check_client_start
    handle check_client_files_download
    handle check_client_files_upload
    handle check_client_files_sync
    handle check_client_files_edit
    handle check_client_tls_generate
    handle check_client_tls_copy
    handle check_client_destroy
    handle cleanup
}

clusterchecks
clientchecks
rm -f ${AEROLAB_CONFIG_FILE}
rm -f strong.conf
handle end
