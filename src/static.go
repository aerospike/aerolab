package main

var loggerServiceName = "AERO-LAB"
var loggerHeader = ""

var aerospikeInstallScript = map[string]string{
	"ubuntu": `apt-get update && apt-get -y install python3-distutils; apt-get -y install libcurl4; apt-get -y install python3-setuptools; apt-get update && apt-get -y install iptables wget dnsutils tcpdump python net-tools vim binutils iproute2 python3 libcurl4-openssl-dev && wget https://github.com/bestmethod/tcconfig-ubuntu-venv/archive/tc.tar.gz && tar -zxvf tc.tar.gz && mv tcconfig-ubuntu-venv-tc /tcconfig && cd /root && tar -zxf installer.tgz && cd aerospike-server-* && ./asinstall && apt-get -y install libldap-2.4-2; apt -y --fix-broken install; apt-get -y install libldap-2.4-2`,
	"el":     `yum -y update && yum -y install iptables wget tcpdump which binutils iproute iproute-tc libcurl-openssl-devel ; yum -y install dnsutils python; yum -y install initscripts; yum -y install redhat-lsb; yum -y install centos-release-scl ; yum -y install rh-python36 ; yum -y install python38 ; cd /root && tar -zxvf installer.tgz && cd aerospike-server-* && ./asinstall`,
	//"el":     `yum -y update && yum -y install iptables wget tcpdump dnsutils python which binutils iproute iproute-tc centos-release-scl && yum install -y rh-python36 && scl enable rh-python36 bash && pip3 install tcconfig && cd /root && tar -zxvf installer.tgz && cd aerospike-server-* && ./asinstall`,
}

var aerospikeInstallScriptDocker = map[string]string{
	"ubuntu": `apt-get update && apt-get -y install python3-distutils; apt-get -y install libcurl4; apt-get -y install python3-setuptools; apt-get update && apt-get -y install iptables wget dnsutils tcpdump python net-tools vim binutils iproute2 python3 libcurl4-openssl-dev && wget https://github.com/bestmethod/tcconfig-ubuntu-venv/archive/tc.tar.gz && tar -zxvf tc.tar.gz && mv /tcconfig-ubuntu-venv-tc /tcconfig && cd /root && tar -zxf installer.tgz && cd aerospike-server-* && ./asinstall && apt-get -y install libldap-2.4-2; apt -y --fix-broken install; apt-get -y install libldap-2.4-2; cat <<'EOF' > /etc/init.d/aerospike
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
yum -y update && yum -y install iptables wget tcpdump which redhat-lsb-core initscripts binutils iproute iproute-tc libcurl-openssl-devel ; yum -y install dnsutils python; yum -y install initscripts; yum -y install redhat-lsb; yum -y install centos-release-scl ; yum install -y rh-python36 ; yum -y install python38 ; cd /root && tar -zxvf installer.tgz && cd aerospike-server-* && ./asinstall; cat <<'EOF' > /etc/init.d/aerospike
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
