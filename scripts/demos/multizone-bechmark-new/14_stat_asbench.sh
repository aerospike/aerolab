. ./configure.sh

# kill asbench
aerolab client attach -n ${CLIENT_NAME} -l all -- pidof asbench
