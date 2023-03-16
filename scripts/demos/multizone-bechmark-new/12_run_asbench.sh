LOAD=$1

if [ "${LOAD}" = "" ]
then
    echo "Usage: $0 i|ru"
    echo "To run insert load, or read-update load"
    exit 1
fi

set -e
. ./configure.sh

# get one node seed IP
NODEIP=$(aerolab cluster list -j |grep -A7 ${CLUSTER_NAME} |grep IpAddress |head -1 |egrep -o '([0-9]{1,3}\.){3}[0-9]{1,3}')
echo "Seed: ${NODEIP}"

# parameters
common_params1="-h ${NODEIP}:3000 -U superman -Pkrypton -n ${NAMESPACE}"
common_params2="-b ${asbench_bin_name} -K ${asbench_start_key} -k ${asbench_end_key} -z ${asbench_threads} -o ${asbench_object}"
common_params3="--socket-timeout ${asbench_socket_timeout} --timeout ${asbench_total_timeout} -B ${asbench_read_policy} --max-retries ${asbench_retries}"

# prepare asbench
if [ "${LOAD}" = "i" ]
then
    for i in `seq 1 ${asbench_per_instance_insert}`
    do
      aerolab client attach -n ${CLIENT_NAME} -l all --detach -- /bin/bash -c "run_asbench ${common_params1} -s n\${NODE}x${i} ${common_params2[@]} -t 0 -w I ${common_params3[@]}"
    done
elif [ "${LOAD}" = "ru" ]
then
    for i in `seq 1 ${asbench_per_instance_load}`
    do
      aerolab client attach -n ${CLIENT_NAME} -l all --detach -- /bin/bash -c "run_asbench ${common_params1[@]} -s n\${NODE}x${i} ${common_params2[@]} -t ${asbench_ru_runtime} -g ${asbench_ru_throughput} -w RU,${asbench_ru_percent} ${common_params3[@]}"
    done
else
    echo "invalid usage"
    exit 1
fi
