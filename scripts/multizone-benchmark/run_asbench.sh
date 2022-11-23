LOAD=$1

if [ "${LOAD}" = "" ]
then
    echo "Usage: $0 i|ru"
    echo "To run insert load, or read-update load"
    exit 1
fi

. configure.sh

# get one node seed IP
NODEIP=$(aerolab cluster list -j |grep -A7 ${NAME} |grep IpAddress |head -1 |egrep -o '([0-9]{1,3}\.){3}[0-9]{1,3}')
echo "Seed: ${NODEIP}"

# prepare asbench
if [ "${LOAD}" = "i" ]
then
    echo "nohup asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${NAMESPACE} -s \$(hostname) --latency -b testbin -K 0 -k 1000000 -z 16 -t 0 -o I1 -w I --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2 >>/var/log/asbench.log 2>&1 &" > asbench.sh
elif [ "${LOAD}" = "ru" ]
then
    echo "nohup asbench -h ${NODEIP}:3000 -U superman -Pkrypton -n ${NAMESPACE} -s \$(hostname) --latency -b testbin -K 0 -k 1000000 -z 16 -t 86400 -g 1000 -o I1 -w RU,80 --socket-timeout 200 --timeout 1000 -B allowReplica --max-retries 2 -d >>/var/log/asbench.log 2>&1 &" > asbench.sh
else
    echo "invalid usage"
    exit 1
fi
aerolab files upload -c -n ${CLIENT_NAME} asbench.sh /opt/run.sh
rm -f asbench.sh

# launch asbench
aerolab client attach -n ${CLIENT_NAME} -l all --detach -- bash /opt/run.sh

