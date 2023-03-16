. ./configure.sh

SOURCE_IPS=()
if [ "$1" != "" ]
then
    SOURCE_IPS=(-i $1)
fi

aerolab config aws list-security-groups |grep AeroLab >/dev/null 2>&1
if [ $? -ne 0 ]
then
    aerolab config aws create-security-groups
fi
aerolab config aws lock-security-groups ${SOURCE_IPS[@]}
