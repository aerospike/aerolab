#!/bin/bash

export AEROLAB_CONFIG_FILE=test.conf
aerolab="./aerolab-macos"
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

function check_template_destroy() {
    ${aerolab} template destroy -d all -i all -v all
    return $?
}

function check_cluster_create() {
    ${aerolab} cluster create -n test -c 3 -v 5.7.* -d ubuntu -B -o strong.conf ${extra}
    return $?
}

function check_cluster_list() {
    ${aerolab} cluster list
    return $?
}

function check_cluster_stop() {
    ${aerolab} cluster stop -n test -l 2
    return $?
}

function check_cluster_start() {
    ${aerolab} cluster start -n test -l 2
    return $?
}

function check_cluster_grow() {
    ${aerolab} cluster grow -n test -c 2 -v 5.7.* -d ubuntu -B -o strong.conf ${extra}
    return $?
}

function check_cluster_destroy() {
    ${aerolab} cluster destroy -n test -l 4 -f || return 1
    ${aerolab} cluster destroy -n test -f
    return $?
}

function check_installer_listversions() {
    ${aerolab} installer list-versions
    return $?
}

function check_installer_download() {
    ${aerolab} installer download -v 6.0.*
    return $?
}

function check_conf_fixmesh() {
    ${aerolab} conf fix-mesh -n test
    return $?
}

function check_aerospike_stop() {
    ${aerolab} aerospike stop -n test -l 2
    return $?
}

function check_aerospike_start() {
    ${aerolab} aerospike start -n test -l 2
    return $?
}

function check_aerospike_restart() {
    ${aerolab} aerospike restart -n test -l 3
    return $?
}

function check_aerospike_upgrade() {
    ${aerolab} aerospike upgrade -n test -l 4,5 -v 6.1.*
    [ $? -ne 0 ] && return 1
    ${aerolab} attach shell -n test -l 4,5 -- rm -f /opt/aerospike/data/bar.dat || return 1
    ${aerolab} cluster stop -n test -l 4,5 || return 1
    ${aerolab} cluster start -n test -l 4,5 || return 1
    return $?
}

function check_roster_show() {
    ${aerolab} roster show -n test -m bar
    return $?
}

function check_roster_apply() {
    ${aerolab} roster apply -n test -m bar
    return $?
}

function check_data_insert() {
    ${aerolab} data insert -m bar -f -u 10 -v 5 -n test -l 3
    return $?
}

function check_data_delete() {
    ${aerolab} data delete -m bar -D -u 10 -v 5 -n test -l 3
    return $?
}

function check_attach_shell() {
    ${aerolab} attach shell -n test -l 2 -- ls
    return $?
}

function check_attach_aql() {
    ${aerolab} attach aql -n test -l 2 -- -c "show sets"
    return $?
}

function check_attach_asadm() {
    ${aerolab} attach asadm -n test -l 2 -- -e info
    return $?
}

function check_attach_asinfo() {
    ${aerolab} attach asinfo -n test -l 2 -- -v service
    return $?
}

function check_files_edit() {
    ${aerolab} files edit -n test -l 1 -e ls /
    return $?
}

function check_files_sync() {
    ${aerolab} files sync -n test -d test -p /etc/aerospike
    return $?
}

function check_files_upload() {
    echo "test" > mytestfile
    ${aerolab} files upload -n test mytestfile /root/
    ret=$?
    rm -f mytestfile
    return $ret
}

function check_files_download() {
    ${aerolab} files download -n test -l 2 /root/mytestfile .
    ret=$?
    [ $ret -ne 0 ] && return $ret
    cat mytestfile
    ret=$?
    rm -f mytestfile
    return $ret
}

function check_logs_show() {
    ${aerolab} logs show -n test ${logsExtra}
    ret=$?
    return $ret
}

function check_logs_get() {
    ${aerolab} logs get -n test ${logsExtra} -d ./log/logs/
    ret=$?
    return $ret
}

function check_net_block() {
    ${aerolab} net block -s test -d test -l 1,2 -i 3,4 -p 3001,3002
    ret=$?
    return $ret
}

function check_net_list() {
    ${aerolab} net list
    ret=$?
    return $ret
}

function check_net_unblock() {
    ${aerolab} net unblock -s test -d test -l 1,2 -i 3,4 -p 3001,3002
    ret=$?
    return $ret
}

function check_net_lossdelay() {
    ${aerolab} net loss-delay -s test -d test -l 1,2 -i 3,4 -a set -p 100ms -L 10% -D -R 1000Kbps
    ret=$?
    [ $ret -ne 0 ] && return $ret
    ${aerolab} net loss-delay -s test -d test -a show
    ret=$?
    [ $ret -ne 0 ] && return $ret
    ${aerolab} net loss-delay -s test -d test -a delall
    return $?
}

function check_template_create() {
    rm -f aerospike-server-enterprise-*.tgz
    ${aerolab} template create -v 5.7.* ${extra}
    [ $? -ne 0 ] && return 1
    check_template_destroy || return 1
    ${aerolab} template create -v 5.7.* ${extra}
    return $?
}

function check_xdr_create() {
    ${aerolab} xdr create-clusters -N testd1,testd2 -C 2 -n tests -c 2 -v 5.7.* -M test,bar -B ${extra} || return 1
}

function check_xdr_connect() {
    ${aerolab} xdr connect -S testd1 -D testd2 -M test,bar || return 1
}

function check_xdr_details() {
    ${aerolab} attach asadm -n tests -l 1 -- -e info || return 1
    ${aerolab} attach asadm -n testd1 -l 1 -- -e info || return 1
    ${aerolab} attach asadm -n testd2 -l 1 -- -e info || return 1
    return 0
}

function check_xdr_crossregion() {
    ${aerolab} config backend -t aws -r us-west-1 || return 1
    ${aerolab} cluster create -n testfar -c 2 -I t3a.medium -E 20 -S sg-01f8d354e1a5e1e1d -U subnet-0494e0b34fa079479 || return 1
    ${aerolab} config backend -t aws -r us-east-1 || return 1
    ${aerolab} xdr connect -S testd2 -D testfar -M test,bar -s us-east-1 -d us-west-1 || return 1
    ${aerolab} config backend -t aws -r us-west-1 || return 1
    ${aerolab} attach shell -n testfar -- asadm -e info || return 1
    ${aerolab} cluster destroy -n testfar || return 1
    ${aerolab} template destroy -v all -d all -i all || return 1
    ${aerolab} config backend -t aws -r us-east-1 || return 1
    ${aerolab} attach shell -n testd2 -- asadm -e info || return 1
}

function check_tls_generate() {
    ${aerolab} tls generate -n tests -t bob11 -c bobca1 || return 1
    ${aerolab} tls generate -n tests -t bob12 -c bobca1 || return 1
    ${aerolab} tls generate -n tests -t bob21 -c bobca2 || return 1
    ${aerolab} tls generate -n tests -t bob22 -c bobca2 || return 1
    return 0
}

function check_tls_copy() {
    ${aerolab} tls copy -s tests -d testd1 -t bob11 || return 1
    ${aerolab} tls copy -s tests -d testd1 -t bob12 || return 1
    ${aerolab} tls copy -s tests -d testd1 -t bob21 || return 1
    ${aerolab} tls copy -s tests -d testd1 -t bob22 || return 1
    ${aerolab} attach shell -n testd1 -l 1 -- find /etc/aerospike/ssl -type f || return 1
    return 0
}

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
rm -f ${AEROLAB_CONFIG_FILE}
rm -f strong.conf
handle end
