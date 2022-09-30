#!/bin/bash

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
