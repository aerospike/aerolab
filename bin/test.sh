aerolab="./aerolab-macos"
cluster="testcluster"
logDir="./log"

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

function check_version() {
    ${aerolab} version
    return $?
}

function check_cluster_destroy() {
    RET=0
    for i in ${cluster}dst1 ${cluster}dst2 ${cluster}src5 ${cluster}src4 ${cluster}srcx5 ${cluster}srcx4 ${cluster}dstx5 ${cluster}dstx4 ${cluster}tls
    do
        ${aerolab} cluster-destroy -f -n ${i} || RET=$?
    done
    ${aerolab} cluster-destroy -f -n ${cluster}cont || RET=$?
    ${aerolab} cluster-destroy -n ${cluster} -f || RET=$?
    return ${RET}
}

function check_cluster_stop() {
    ${aerolab} cluster-stop -n ${cluster}
    return $?
}

function check_cluster_start() {
    ${aerolab} cluster-start -n ${cluster}
    return $?
}

function check_cluster_list() {
    ${aerolab} cluster-list
    return $?
}

function check_cluster_grow() {
    ${aerolab} cluster-grow -n ${cluster} -c 2 -m mesh -v 4.9.0.32
    return $?
}

function check_make_basic() {
    ${aerolab} make-cluster -n ${cluster} -c 2 -m mesh
    return $?
}

function check_stop_aerospike() {
    ${aerolab} stop-aerospike -n ${cluster}
    return $?
}

function check_start_aerospike() {
    ${aerolab} start-aerospike -n ${cluster}
    return $?
}

function check_restart_aerospike() {
    ${aerolab} restart-aerospike -n ${cluster}
    return $?
}

function check_upgrade_aerospike() {
    ${aerolab} upgrade-aerospike -n ${cluster} -l 3,4 -s n
    return $?
}

function check_conf_fix_mesh() {
    ${aerolab} conf-fix-mesh -n ${cluster}
    return $?
}

function check_nuke_templates() {
    ${aerolab} nuke-template -v all -d all -i all
    return $?
}

function check_node_attach() {
    ${aerolab} node-attach -n ${cluster} -- asinfo -v service
    return $?
}

function check_aql() {
    ${aerolab} aql -n ${cluster} -- -c "show sets"
    return $?
}

function check_asinfo() {
    ${aerolab} asinfo -n ${cluster} -- -v service
    return $?
}

function check_asadm() {
    ${aerolab} asadm -n ${cluster} -- -e info
    return $?
}

function check_schelp() {
    ${aerolab} sc-help
    return $?
}

function check_xdr_connect_5_auto() {
	${aerolab} make-cluster -n ${cluster}dst1 -c 2 -m mesh || return $?
	${aerolab} make-cluster -n ${cluster}dst2 -c 2 -m mesh || return $?
	${aerolab} make-cluster -n ${cluster}src5 -c 2 -m mesh || return $?
	${aerolab} xdr-connect -s ${cluster}src5 -d ${cluster}dst1,${cluster}dst2 -m test,bar || return $?
	${aerolab} restart-aerospike -n ${cluster}src5 || return $?
	return 0
}

function check_xdr_connect_4_auto() {
	${aerolab} make-cluster -n ${cluster}src4 -c 2 -m mesh -v 4.9.0.32 || return $?
	${aerolab} xdr-connect -s ${cluster}src4 -d ${cluster}dst1,${cluster}dst2 -m test,bar || return $?
	${aerolab} restart-aerospike -n ${cluster}src4 || return $?
	return 0
}

function check_make_xdr_clusters_5() {
	${aerolab} make-xdr-clusters -s ${cluster}srcx5 -x ${cluster}dstx5 -m test,bar || return $?
    ${aerolab} restart-aerospike -n ${cluster}srcx5 || return $?
	return $?
}

function check_make_xdr_clusters_4() {
	${aerolab} make-xdr-clusters -s ${cluster}srcx4 -x ${cluster}dstx4 -m test,bar -v 4.9.0.32 || return $?
    ${aerolab} restart-aerospike -n ${cluster}srcx4 || return $?
	return $?
}

function pause() {
    sleep 5
}

function cleanup() {
    check_cluster_destroy
    check_nuke_templates
}

function check_insert_data() {
	${aerolab} insert-data -a 1 -z 10000 -m test -s myset -b static:mybin -f -u 5 -T 0 -E UPDATE -n ${cluster}src5
    return $?
}

function check_delete_data() {
	${aerolab} delete-data -a 1 -z 7000 -m test -s myset -u 5 -D -n ${cluster}src5
    return $?
}

function check_upload_download() {
    date > log/timestamp.out
    ${aerolab} upload -n ${cluster} -i log/timestamp.out -o /root/timestamp || return $?
    ${aerolab} download -n ${cluster} -i /root/timestamp -o log/timestamp.in || return $?
    diff log/timestamp.out log/timestamp.in
    return $?
}

function check_deploy_container() {
    ${aerolab} deploy-container -n ${cluster}cont
    return $?
}

function check_get_log() {
    mkdir -p ${logDir}/node_logs
    ${aerolab} get-logs -n ${cluster} -o ${logDir}/node_logs/ || return $?
    for i in ${cluster}dst1 ${cluster}dst2 ${cluster}src5 ${cluster}src4 ${cluster}srcx5 ${cluster}srcx4 ${cluster}dstx5 ${cluster}dstx4 ${cluster}tls
    do
        ${aerolab} get-logs -n ${i} -o ${logDir}/node_logs/ || return $?
    done
    return 0
}

function check_tls() {
    ${aerolab} make-cluster -o $(pwd)/../templates/tls.conf -c 2 -n ${cluster}tls || return $?
    ${aerolab} gen-tls-certs -n ${cluster}tls || return $?
    ${aerolab} restart-aerospike -n ${cluster}tls || return $?
    return 0
}

function check_tls_connect() {
    ${aerolab} aql -n ${cluster}tls -- --tls-enable --tls-cafile=/etc/aerospike/ssl/tls1/cacert.pem -h 127.0.0.1:tls1:4333 -c "show sets" || return $?
    return 0
}

function check_tls_copy() {
    ${aerolab} cluster-grow -o $(pwd)/../templates/tls.conf -c 2 -n ${cluster}tls || return $?
    ${aerolab} copy-tls-certs -s ${cluster}tls -l 1 -d ${cluster}tls -a 3,4 || return $?
    return 0
}

function check_net_block() {
    ${aerolab} net-block -s ${cluster}src5 -d ${cluster}dst1 -t drop -p 3000 -b input -M random -P 0.5
    return $?
}

function check_net_list() {
    ${aerolab} net-list
    return $?
}

function check_net_unblock() {
    ${aerolab} net-unblock -s ${cluster}src5 -d ${cluster}dst1 -t drop -p 3000 -b input -M random -P 0.5
    return $?
}

function check_tc_set() {
    ${aerolab} net-loss-delay -s ${cluster}src5 -d ${cluster}dst2 -a set -p 50ms -L 3% -D -R 100Kbps
    return $?
}

function check_tc_show() {
    ${aerolab} net-loss-delay -s ${cluster}src5 -d ${cluster}dst2 -a show -D
    return $?
}

function check_tc_del() {
    ${aerolab} net-loss-delay -s ${cluster}src5 -d ${cluster}dst2 -a del -D
    return $?
}

function end() {
    return 0
}

handle cleanup
handle check_version
handle check_make_basic
handle check_stop_aerospike
handle check_cluster_stop
handle check_cluster_start
handle check_start_aerospike
handle check_cluster_grow
handle check_conf_fix_mesh
handle check_cluster_list
handle check_upgrade_aerospike
handle check_restart_aerospike
handle check_schelp
handle pause
handle check_node_attach
handle check_aql
handle check_asinfo
handle check_asadm
handle check_xdr_connect_5_auto
handle check_xdr_connect_4_auto
handle check_make_xdr_clusters_5
handle check_make_xdr_clusters_4
handle check_upload_download
handle check_deploy_container
handle check_tls
handle pause
handle check_tls_connect
handle check_tls_copy
handle pause
handle check_tls_connect
handle check_net_block
handle check_net_list
handle check_insert_data
handle pause
handle pause
handle check_net_unblock
handle check_tc_set
handle check_tc_show
handle check_delete_data
handle check_tc_del
handle pause
handle pause
handle pause
handle pause
handle check_get_log
handle check_cluster_destroy
handle check_nuke_templates
handle end
