set -e
DOCKER=%s
ISDEB=%s
if [ ${ISDEB} -eq 0 ]
then
    yum -y --allowerasing --nobest install curl
else
    apt update && apt -y install curl
fi
curl -L -o /tmp/installer.%s '%s'
[ -f /tmp/installer.deb ] && apt -y install openjdk-21-jdk-headless /tmp/installer.deb
if [ -f /tmp/installer.rpm ]; then
    yum -y install epel-release
    yum -y install java-latest-openjdk
    alternatives --set java java-latest-openjdk.x86_64
    yum -y localinstall /tmp/installer.rpm
fi
if [ ${DOCKER} -eq 0 ]
then
    systemctl enable aerospike-proximus
else
    mkdir -p /opt/autoload
    echo "nohup /opt/aerospike-proximus/bin/aerospike-proximus -f /etc/aerospike-proximus/aerospike-proximus.yml >>/var/log/aerospike-proximus.out.log 2>&1 &" > /opt/autoload/10-proximus
    chmod 755 /opt/autoload/10-proximus
fi
