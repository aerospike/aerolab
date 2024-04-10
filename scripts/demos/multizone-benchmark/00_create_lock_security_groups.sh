. ./configure.sh

SOURCE_IPS=()
if [ "$1" != "" ]
then
    SOURCE_IPS=(-i $1)
fi

if [ "${BACKEND}" = "aws" ]
then
    aerolab config aws list-security-groups |grep AeroLab >/dev/null 2>&1
    if [ $? -ne 0 ]
    then
        aerolab config aws create-security-groups
    fi
    aerolab config aws lock-security-groups ${SOURCE_IPS[@]}
elif [ "${BACKEND}" = "gcp" ]
    aerolab config gcp list-firewall-rules |grep aerolab-managed-external >/dev/null 2>&1
    if [ $? -ne 0 ]
    then
        aerolab config gcp create-firewall-rules
    fi
    aerolab config gcp lock-firewall-rules ${SOURCE_IPS[@]}
else
    echo "this is only available for gcp/aws backend, ignoring"
fi
