set -e
. ./configure.sh
echo
echo "Default user:pass is admin:admin"
echo
nip=$(aerolab client list |grep ${AMS_NAME} |egrep -o '([0-9]{1,3}\.){3}[0-9]{1,3}' |tail -1)
echo "grafana: http://${nip}:3000"
