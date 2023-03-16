. ./configure.sh
[ $# -gt 0 ] && aerolab attach asadm -n ${CLUSTER_NAME} -- "$@"
[ $# -eq 0 ] && aerolab attach asadm -n ${CLUSTER_NAME}
exit 0
