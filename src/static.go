package main

var loggerServiceName = "AERO-LAB"
var loggerHeader = ""

var aerospikeInstallScript = map[string]string{
	"ubuntu": `apt-get update && apt-get -y install python3-distutils; apt-get -y install python3-setuptools; apt-get update && apt-get -y install iptables wget dnsutils tcpdump python net-tools vim binutils iproute2 python3 && wget https://github.com/bestmethod/tcconfig-ubuntu-venv/archive/tc.tar.gz && tar -zxvf tc.tar.gz && mv tcconfig-ubuntu-venv-tc /tcconfig && cd /root && tar -zxf installer.tgz && cd aerospike-server-* && ./asinstall && apt-get -y install libldap-2.4-2; apt -y --fix-broken install; apt-get -y install libldap-2.4-2`,
	"el":     `yum -y update && yum -y install iptables wget tcpdump which binutils iproute iproute-tc ; yum -y install dnsutils python; yum -y install initscripts; yum -y install redhat-lsb; yum -y install centos-release-scl ; yum -y install rh-python36 ; yum -y install python38 ; cd /root && tar -zxvf installer.tgz && cd aerospike-server-* && ./asinstall`,
	//"el":     `yum -y update && yum -y install iptables wget tcpdump dnsutils python which binutils iproute iproute-tc centos-release-scl && yum install -y rh-python36 && scl enable rh-python36 bash && pip3 install tcconfig && cd /root && tar -zxvf installer.tgz && cd aerospike-server-* && ./asinstall`,
}

var aerospikeInstallScriptDocker = map[string]string{
	"ubuntu": `apt-get update && apt-get -y install python3-distutils; apt-get -y install python3-setuptools; apt-get update && apt-get -y install iptables wget dnsutils tcpdump python net-tools vim binutils iproute2 python3 && wget https://github.com/bestmethod/tcconfig-ubuntu-venv/archive/tc.tar.gz && tar -zxvf tc.tar.gz && mv /tcconfig-ubuntu-venv-tc /tcconfig && cd /root && tar -zxf installer.tgz && cd aerospike-server-* && ./asinstall && apt-get -y install libldap-2.4-2; apt -y --fix-broken install; apt-get -y install libldap-2.4-2; cat <<'EOF' > /etc/init.d/aerospike
#!/bin/sh
# Start/stop the aerospike daemon.
#
### BEGIN INIT INFO
# Provides:          aerospike
# Required-Start:    $remote_fs $syslog $time
# Required-Stop:     $remote_fs $syslog $time
# Should-Start:      $network $named slapd autofs ypbind nscd nslcd winbind
# Should-Stop:       $network $named slapd autofs ypbind nscd nslcd winbind
# Default-Start:     2 3 4 5
# Default-Stop:
# Short-Description: Aerospike
# Description:       Aerospike
### END INIT INFO

PATH=/bin:/usr/bin:/sbin:/usr/sbin
DESC="aerospike daemon"
NAME=aerospike
DAEMON=/usr/bin/asd
PIDFILE=/var/run/asd.pid
SCRIPTNAME=/etc/init.d/"$NAME"

test -f $DAEMON || exit 0

. /lib/lsb/init-functions

case "$1" in
start)  log_daemon_msg "Starting aerospike" "aerospike"
        start_daemon -p $PIDFILE $DAEMON $EXTRA_OPTS
        log_end_msg $?
        ;;
stop)   log_daemon_msg "Stopping aerospike" "aerospike"
        pkill asd
        RETVAL=$?
        if [ $RETVAL -ne 0 ]; then pkill -9 asd; fi 
        log_end_msg $RETVAL
        ;;
restart) log_daemon_msg "Restarting aerospike" "aerospike"
        $0 stop
        $0 start
        ;;
coldstart)  log_daemon_msg "Starting aerospike" "aerospike"
        start_daemon -p $PIDFILE $DAEMON --cold-start $EXTRA_OPTS
        log_end_msg $?
        ;;
status)
        pidof asd
        if [ $? -ne 0 ]; then echo "Aerospike stopped"; else echo "Aerospike running"; fi
        ;;
*)      log_action_msg "Usage: /etc/init.d/aerospike {start|stop|status|restart|coldstart}"
        exit 2
        ;;
esac
exit 0
EOF
chmod 755 /etc/init.d/aerospike
`,
	"el": `set -o xtrace
yum -y update && yum -y install iptables wget tcpdump which redhat-lsb-core initscripts binutils iproute iproute-tc ; yum -y install dnsutils python; yum -y install initscripts; yum -y install redhat-lsb; yum -y install centos-release-scl ; yum install -y rh-python36 ; yum -y install python38 ; cd /root && tar -zxvf installer.tgz && cd aerospike-server-* && ./asinstall; cat <<'EOF' > /etc/init.d/aerospike
#!/bin/sh
# Start/stop the aerospike daemon.
#
### BEGIN INIT INFO
# Provides:          aerospike
# Required-Start:    $remote_fs $syslog $time
# Required-Stop:     $remote_fs $syslog $time
# Should-Start:      $network $named slapd autofs ypbind nscd nslcd winbind
# Should-Stop:       $network $named slapd autofs ypbind nscd nslcd winbind
# Default-Start:     2 3 4 5
# Default-Stop:
# Short-Description: Aerospike
# Description:       Aerospike
### END INIT INFO

PATH=/bin:/usr/bin:/sbin:/usr/sbin
DESC="aerospike daemon"
NAME=aerospike
DAEMON=/usr/bin/asd
PIDFILE=/var/run/asd.pid
SCRIPTNAME=/etc/init.d/"$NAME"

test -f $DAEMON || exit 0

. /lib/lsb/init-functions

case "$1" in
start)  log_success_msg "Starting aerospike" "aerospike"
        start_daemon -p $PIDFILE $DAEMON $EXTRA_OPTS
        log_success_msg $?
        ;;
stop)   log_success_msg "Stopping aerospike" "aerospike"
		pkill asd
		sleep 1
		if [ $? -ne 0 ]; then pkill -9 asd; fi
        log_success_msg $RETVAL
        ;;
restart) log_success_msg "Restarting aerospike" "aerospike"
        $0 stop
        $0 start
        ;;
coldstart)  log_success_msg "Starting aerospike" "aerospike"
        start_daemon -p $PIDFILE $DAEMON --cold-start $EXTRA_OPTS
        log_success_msg $?
        ;;
status)
        status_of_proc -p $PIDFILE $DAEMON $NAME && exit 0 || exit $?
        ;;
*)      log_success_msg "Usage: /etc/init.d/aerospike {start|stop|status|restart|coldstart}"
        exit 2
        ;;
esac
exit 0
EOF
chmod 755 /etc/init.d/aerospike
`,
}

var aerospikeInitd = `#!/bin/sh
# Start/stop the aerospike daemon.
#
### BEGIN INIT INFO
# Provides:          aerospike
# Required-Start:    $remote_fs $syslog $time
# Required-Stop:     $remote_fs $syslog $time
# Should-Start:      $network $named slapd autofs ypbind nscd nslcd winbind
# Should-Stop:       $network $named slapd autofs ypbind nscd nslcd winbind
# Default-Start:     2 3 4 5
# Default-Stop:
# Short-Description: Aerospike
# Description:       Aerospike
### END INIT INFO

PATH=/bin:/usr/bin:/sbin:/usr/sbin
DESC="aerospike daemon"
NAME=aerospike
DAEMON=/usr/bin/asd
PIDFILE=/var/run/asd.pid
SCRIPTNAME=/etc/init.d/"$NAME"

test -f $DAEMON || exit 0

. /lib/lsb/init-functions

case "$1" in
start)  log_daemon_msg "Starting aerospike" "aerospike"
        start_daemon -p $PIDFILE $DAEMON $EXTRA_OPTS
        log_end_msg $?
        ;;
stop)   log_daemon_msg "Stopping aerospike" "aerospike"
        killproc -p $PIDFILE $DAEMON
        RETVAL=$?
        [ $RETVAL -eq 0 ] && [ -e "$PIDFILE" ] && rm -f $PIDFILE
        log_end_msg $RETVAL
        ;;
restart) log_daemon_msg "Restarting aerospike" "aerospike"
        $0 stop
        $0 start
        ;;
coldstart)  log_daemon_msg "Starting aerospike" "aerospike"
        start_daemon -p $PIDFILE $DAEMON --cold-start $EXTRA_OPTS
        log_end_msg $?
        ;;
status)
        status_of_proc -p $PIDFILE $DAEMON $NAME && exit 0 || exit $?
        ;;
*)      log_action_msg "Usage: /etc/init.d/aerospike {start|stop|status|restart|coldstart}"
        exit 2
        ;;
esac
exit 0
`

var clientInstallPre = `#!/bin/bash

set -o xtrace

# pre-requisites
cd /root
apt-get update
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
# 18.04: apt-get -y install git wget curl gcc make wget unzip libssl1.0-dev zlib1g-dev iptables binutils
apt-get -y install git wget curl gcc make wget unzip zlib1g-dev iptables binutils libssl-dev libssl1.0.0 vim
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi

# c sharp.net
### n/a

# c libevent
### n/a

# php
### n/a

# perl
### n/a

`

var clientInstallScript = map[string]string{
	"go": `wget https://dl.google.com/go/go1.10.2.linux-amd64.tar.gz
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
tar -C /usr/local -xzf go1.10.2.linux-amd64.tar.gz
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
echo 'export PATH=$PATH:/usr/local/go/bin' >> /root/.bashrc
/usr/local/go/bin/go get github.com/aerospike/aerospike-client-go
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
`,
	"python": `apt-get -y install python3 python3-pip python python-pip
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
pip3 install --upgrade pip
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
sleep 1
cat <<EOF > /usr/bin/pip3
#!/usr/bin/python3
# -*- coding: utf-8 -*-
import re
import sys
from pip._internal import main
if __name__ == '__main__':
   sys.argv[0] = re.sub(r'(-script\.pyw|\.exe)?$', '', sys.argv[0])
   sys.exit(main())
EOF
sleep 1
pip3 install --upgrade wheel setuptools
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
pip3 install aerospike
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
`,
	"c": `mkdir aerospike-c-libs
cd aerospike-c-libs
wget https://www.aerospike.com/download/client/c/latest/artifact/ubuntu16
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
wget https://www.aerospike.com/download/client/c/latest/artifact/ubuntu16-libuv
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
wget https://www.aerospike.com/download/client/c/latest/artifact/ubuntu16-libev
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
for i in ubuntu16 ubuntu16-libev ubuntu16-libuv; do tar -zxvf $i; rm -f $i; done
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
cd ~
`,
	"node": `apt-get -y install npm nodejs node-gyp nodejs-dev nodejs-legacy
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
npm install aerospike
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
`,
	"rust": `curl https://sh.rustup.rs -sSf > rust.sh
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
chmod 755 rust.sh
./rust.sh -y
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
rm -f rust.sh
`,
	"ruby": `apt-get -y install ruby-full
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
gem install aerospike
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
`,
	"java": `apt-get -y install openjdk-8-jdk maven gradle
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
gradle -v
mvn --version
cd /root
mkdir java
cd java
wget https://www.aerospike.com/download/client/java/latest/artifact/tgz
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
tar -zxvf tgz
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
rm -f tgz
wget https://www.aerospike.com/download/client/java/latest/artifact/jar
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
wget https://www.aerospike.com/download/client/java/latest/artifact/jar_dependencies
if [ $? -ne 0 ]; then echo "FAIL"; exit 1; fi
`,
}

var clientInstallPost = `
#### notes
set +o xtrace
echo
echo "----------------------------------------------------------------------"
echo
echo "golang version 1.10.2, aerospike module installed via go get, user root"
echo
echo "python3 installed with latest aerospike module via pip, use pip3, python3"
echo
echo "c libs downloaded to /root/aerospike-c-libs/, install the one you want (standard, libev, libuv) using dpkg"
echo
echo "nodejs installed, aerospike installed in /root via npm"
echo
echo "rust installed, aerospike installation happens in projects (see documentation)"
echo
echo "ruby installed, aerospike module installed via gem"
echo
echo "java installed (openjdk-8-jdk), installed maven and gradle for package management"
echo "aerospike java sdk with demos and jar files downloaded to /root/java"
echo
echo "this script does not install c sharp/.net, c libevent, php, perl"
echo
echo "----------------------------------------------------------------------"
echo
echo "Example:"
echo "cd /root/java/aerospike-client-java-*/benchmarks"
echo "mvn package"
echo "./run_benchmarks -h <ip_of_cluster_node>"
echo
`
