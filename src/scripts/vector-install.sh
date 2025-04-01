set -e
DOCKER=%s
ISDEB=%s
ASVEC_AMD=%s
ASVEC_ARM=%s
VECSEED=%s
if [ ${ISDEB} -eq 0 ]
then
    yum -y --allowerasing --nobest install curl
    cat << EOF > /etc/yum.repos.d/aerospike.repo
[aerospike]
name=Aerospike
baseurl=https://aerospike.jfrog.io/artifactory/rpm
enabled=1
gpgcheck=1
gpgkey=https://aerospike.jfrog.io/artifactory/api/security/keypair/aerospike/public
autorefresh=1
type=rpm-md
EOF
    yum -y install epel-release
    yum -y install java-latest-openjdk
    alternatives --set java java-latest-openjdk.x86_64
    yum -y install aerospike-vector-search unzip git
else
    apt update && apt -y install curl wget gpg openjdk-21-jdk-headless unzip
    wget -qO - https://aerospike.jfrog.io/artifactory/api/security/keypair/aerospike/public | gpg --dearmor -o /usr/share/keyrings/aerospike.gpg
    echo 'deb [signed-by=/usr/share/keyrings/aerospike.gpg] https://aerospike.jfrog.io/artifactory/deb stable main' > /etc/apt/sources.list.d/aerospike.list
    apt update && apt -y install aerospike-vector-search git
fi
if [ ${DOCKER} -eq 0 ]
then
    systemctl enable aerospike-vector-search
else
    mkdir -p /opt/autoload
    echo "nohup /opt/aerospike-vector-search/bin/aerospike-vector-search -f /etc/aerospike-vector-search/aerospike-vector-search.yml >>/var/log/aerospike-vector-search.out.log 2>&1 &" > /opt/autoload/10-vector
    chmod 755 /opt/autoload/10-vector
fi

UN=$(uname -m)
arch=amd64
[ "$UN" = "aarch64" ] && arch=arm64
[ "$UN" = "arm64" ] && arch=arm64
if [ "$arch" = "amd64" ]; then
  wget -O /tmp/asvec.zip $ASVEC_AMD
else
  wget -O /tmp/asvec.zip $ASVEC_ARM
fi
unzip /tmp/asvec.zip
mv asvec /usr/local/bin/

if [ "${VECSEED}" != "" ]
then
mkdir -p /etc/aerospike
cat <<EOF > /etc/aerospike/asvec.yml
default:
  host: ${VECSEED}
  #seeds: ${VECSEED},${VECSEED}
EOF
fi

cd /root
set +e
git clone https://github.com/aerospike/aerospike-vector.git || exit 0
if [ ${ISDEB} -eq 2 ]
then
  ln -fs /usr/share/zoneinfo/America/Los_Angeles /etc/localtime
  DEBIAN_FRONTEND=noninteractive apt -y install python3 python3-pip vim
  python3 -m pip install --break-system-packages aerospike-vector-search
fi
