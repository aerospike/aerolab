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

# prepare asbench
if [ "${LOAD}" = "i" ]
then
    for i in `seq 1 ${asbench_per_instance_insert}`
    do
      aerolab client attach -n ${CLIENT_NAME} -l all --detach -- /bin/bash -c "run_asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${namespace} -s n\${NODE}x${asbench_per_instance_insert} -b testbin -K 0 -k 1000000 -z 16 -t 0 -o I1 -w I --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2"
    done
elif [ "${LOAD}" = "ru" ]
then
    for i in `seq 1 ${asbench_per_instance_load}`
    do
        aerolab client attach -n ${CLIENT_NAME} -l all --detach -- /bin/bash -c "run_asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${namespace} -s n\${NODE}x${asbench_per_instance_load} -b testbin -K 0 -k 1000000 -z 16 -t 86400 -g 1000 -o I1 -w RU,80 --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2"
    done
else
    echo "invalid usage"
    exit 1
fi
