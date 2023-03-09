cluster_name="robert"
client_name="glonek"
ams_name="ams"
namespace="test"
nodes="2"
clients="2"
backend="aws"
aws_region="ca-central-1"
server_instance="t3a.large"
client_instance="t3a.medium"
ams_instance="t3a.large"

aerolab config backend -t ${backend} -r ${aws_region} || exit 1

if [ "$1" = "setup" ]
then
    set -e
    aerolab cluster create -c ${nodes} -n ${cluster_name} -I ${server_instance}
    aerolab cluster add exporter -n ${cluster_name}
    aerolab client create ams -n ${ams_name} -s ${cluster_name} -I ${ams_instance}
    aerolab client create tools -n ${client_name} -c ${clients} -I ${client_instance}
    aerolab client configure tools -l all -n ${client_name} --ams ${ams_name}
    aerolab client list
elif [ "$1" = "insert" ]
then
    set -e
    NODEIP=$(aerolab cluster list -j |grep -A7 ${cluster_name} |grep IpAddress |head -1 |egrep -o '([0-9]{1,3}\.){3}[0-9]{1,3}')
    echo "Seed: ${NODEIP}"
    aerolab client attach -n ${client_name} -l all --detach -- /bin/bash -c "run_asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${namespace} -s n\${NODE}x\$(pidof asbench |wc -l) -b testbin -K 0 -k 1000000 -z 16 -t 0 -o I1 -w I --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2"
elif [ "$1" = "status" ]
then
    aerolab attach client -n ${client_name} -l all -- pidof asbench
elif [ "$1" = "load" ]
then
    set -e
    NODEIP=$(aerolab cluster list -j |grep -A7 ${cluster_name} |grep IpAddress |head -1 |egrep -o '([0-9]{1,3}\.){3}[0-9]{1,3}')
    echo "Seed: ${NODEIP}"
    aerolab client attach -n ${client_name} -l all --detach -- /bin/bash -c "run_asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${namespace} -s n\${NODE}x\$(pidof asbench |wc -l) -b testbin -K 0 -k 1000000 -z 16 -t 86400 -g 1000 -o I1 -w RU,80 --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2"
elif [ "$1" = "load-after-insert" ]
then
    RET=0 ; while [ $RET -eq 0 ]; do $0 status; RET=$?; sleep 5; done; $0 load
elif [ "$1" = "stopbench" ]
then
    set -e
    aerolab client attach -n ${client_name} -l all -- pkill asbench
elif [ "$1" = "kill" ]
then
    aerolab cluster destroy -f -n ${cluster_name}
    aerolab client destroy -f -n ${client_name}
    aerolab client destroy -f -n ${ams_name}
else
    echo "Usage: $0 [setup | insert | status | load | load-after-insert | stopbench | kill]"
    echo ""
    echo "Commands:"
    echo "  setup             - deploy cluster, clients and AMS in docker or aws"
    echo "  insert            - run insert load, run this multiple times to run many asbench per client instance"
    echo "  status            - find out if asbench is still running"
    echo "  load              - run a read-update load, run multiple times to run many asbench per client instance"
    echo "  load-after-insert - wait for insert load to finish and run the read-update load"
    echo "  stopbench         - stop asbench on all machines"
    echo "  kill              - destroy the stack"
fi
