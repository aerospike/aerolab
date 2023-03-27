package main

import "strings"

var aerospikeInstallScript = make(map[string]string) //docker:centos:7 = script; or aws:ubuntu:20.04 = script

func init() {

	aerospikeInstallScript["aws:ubuntu:22.04"] = `#!/bin/bash
sed -i.bak "/#\$nrconf{restart} = .*/s/.*/\$nrconf{restart} = 'a';/" /etc/needrestart/needrestart.conf || echo "No sed"
cat <<'EOF' > /usr/local/bin/early.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
cat <<'EOF' > /usr/local/bin/late.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
chmod 755 /usr/local/bin/early.sh
chmod 755 /usr/local/bin/late.sh
mkdir -p /etc/systemd/system/aerospike.service.d
cat <<'EOF' > /etc/systemd/system/aerospike.service.d/aerolab-early-late.conf
[Service]
ExecStartPre=/bin/bash /usr/local/bin/early.sh
ExecStopPost=/bin/bash /usr/local/bin/late.sh
EOF
chmod 755 /etc/systemd/system/aerospike.service.d/aerolab-early-late.conf
systemctl daemon-reload
export DEBIAN_FRONTEND=noninteractive
apt-get update || exit 1
apt-get -y install python3-distutils || apt-get update && apt-get -y install python3-distutils || exit 1
apt-get -y install libcurl4 || apt-get update && apt-get -y install libcurl4 || exit 1
apt-get -y install ldap-utils || apt-get update && apt-get -y install ldap-utils || exit 1
apt-get -y install python3-setuptools || apt-get update && apt-get -y install python3-setuptools || exit 1
apt-get -y install python
apt-get -y install iptables wget dnsutils tcpdump net-tools vim binutils iproute2 python3 libcurl4-openssl-dev less || apt-get update && apt-get -y install iptables wget dnsutils tcpdump net-tools vim binutils iproute2 python3 libcurl4-openssl-dev less || exit 1
apt-get -y install dnsutils iputils-ping telnet netcat sysstat vim
wget https://github.com/bestmethod/tcconfig-ubuntu-venv/archive/tc.tar.gz || exit 1
tar -zxvf tc.tar.gz || exit 1
mv tcconfig-ubuntu-venv-tc /tcconfig || exit 1
mv /tcconfig/lib/* /tcconfig/lib/python$(python3 --version |egrep -o '[0-9]+\.[0-9]+')
cd /root && tar -zxf installer.tgz || exit 1
cd aerospike-server-* ; ./asinstall || exit 1
apt-get -y install libldap-2.4-2 ; apt -y --fix-broken install ; apt-get -y install libldap-2.4-2
cat <<'EOF' > /etc/apt/apt.conf.d/local
Dpkg::Options {
	"--force-confdef";
	"--force-confold";
}
EOF
cat <<'EOF' > /etc/dpkg/dpkg.cfg.d/local
force-confdef
force-confold
EOF
`

	aerospikeInstallScript["aws:ubuntu:20.04"] = aerospikeInstallScript["aws:ubuntu:22.04"]
	aerospikeInstallScript["aws:ubuntu:18.04"] = aerospikeInstallScript["aws:ubuntu:22.04"]

	aerospikeInstallScript["aws:centos:7"] = `#!/bin/bash
set -o xtrace
cat <<'EOF' > /usr/local/bin/early.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
cat <<'EOF' > /usr/local/bin/late.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
chmod 755 /usr/local/bin/early.sh
chmod 755 /usr/local/bin/late.sh
mkdir -p /etc/systemd/system/aerospike.service.d
cat <<'EOF' > /etc/systemd/system/aerospike.service.d/aerolab-early-late.conf
[Service]
ExecStartPre=/bin/bash /usr/local/bin/early.sh
ExecStopPost=/bin/bash /usr/local/bin/late.sh
EOF
chmod 755 /etc/systemd/system/aerospike.service.d/aerolab-early-late.conf
systemctl daemon-reload
yum -y update || exit 1
yum -y install iptables wget tcpdump which redhat-lsb-core initscripts binutils iproute iproute-tc libcurl-openssl-devel || exit 1
yum -y install dnsutils || yum -y install bind-utils
yum -y install python
yum -y install initscripts || exit 1
yum -y install redhat-lsb || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
yum -y install centos-release-scl ; yum install -y rh-python36 ; yum -y install python38 || yum -y install python36
cd /root && tar -zxvf installer.tgz || exit 1
cd aerospike-server-* ; ./asinstall || exit 1
`

	aerospikeInstallScript["aws:centos:8"] = `#!/bin/bash
set -o xtrace
cat <<'EOF' > /usr/local/bin/early.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
cat <<'EOF' > /usr/local/bin/late.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
chmod 755 /usr/local/bin/early.sh
chmod 755 /usr/local/bin/late.sh
mkdir -p /etc/systemd/system/aerospike.service.d
cat <<'EOF' > /etc/systemd/system/aerospike.service.d/aerolab-early-late.conf
[Service]
ExecStartPre=/bin/bash /usr/local/bin/early.sh
ExecStopPost=/bin/bash /usr/local/bin/late.sh
EOF
chmod 755 /etc/systemd/system/aerospike.service.d/aerolab-early-late.conf
systemctl daemon-reload
dnf --disablerepo '*' --enablerepo=extras swap centos-linux-repos centos-stream-repos -y && dnf distro-sync -y || exit 1
yum -y update || exit 1
yum -y install iptables wget tcpdump which redhat-lsb-core initscripts binutils iproute iproute-tc libcurl-devel || exit 1
yum -y install dnsutils || yum -y install bind-utils
yum -y install python
yum -y install initscripts || exit 1
yum -y install redhat-lsb || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
yum -y install centos-release-scl ; yum install -y rh-python36 ; yum -y install python38 || yum -y install python36
cd /root && tar -zxvf installer.tgz || exit 1
cd aerospike-server-* ; ./asinstall || exit 1
rm -f /etc/systemd/system/sshd-keygen\@.service.d/disable-sshd-keygen-if-cloud-init-active.conf
systemctl daemon-reload
`
	//systemctl enable --now cockpit.socket; echo b0bTheBuilder |passwd --stdin root;  echo b0bTheBuilder |passwd --stdin centos

	aerospikeInstallScript["aws:amazon:2"] = `#!/bin/bash
set -i xtrace
cat <<'EOF' > /usr/local/bin/early.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
cat <<'EOF' > /usr/local/bin/late.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
chmod 755 /usr/local/bin/early.sh
chmod 755 /usr/local/bin/late.sh
mkdir -p /etc/systemd/system/aerospike.service.d
cat <<'EOF' > /etc/systemd/system/aerospike.service.d/aerolab-early-late.conf
[Service]
ExecStartPre=/bin/bash /usr/local/bin/early.sh
ExecStopPost=/bin/bash /usr/local/bin/late.sh
EOF
chmod 755 /etc/systemd/system/aerospike.service.d/aerolab-early-late.conf
systemctl daemon-reload
yum -y update || exit 1
yum -y install iptables wget tcpdump which redhat-lsb-core initscripts binutils iproute iproute-tc libcurl-openssl-devel || exit 1
yum -y install dnsutils || yum -y install bind-utils
yum -y install python
yum -y install initscripts || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
yum -y install centos-release-scl ; yum install -y rh-python36 ; yum -y install python38 || yum -y install python36
cd /root && tar -zxvf installer.tgz || exit 1
cd aerospike-server-* ; ./asinstall || exit 1
`

	aerospikeInstallScript["docker:ubuntu:22.04"] = `#!/bin/bash
export DEBIAN_FRONTEND=noninteractive
apt-get update || exit 1
apt-get -y install python3-distutils || exit 1
apt-get -y install libcurl4 || exit 1
apt-get -y install ldap-utils || exit 1
apt-get -y install python3-setuptools || exit 1
apt-get -y install python
apt-get -y install iptables wget dnsutils tcpdump net-tools vim binutils iproute2 python3 libcurl4-openssl-dev less || exit 1
apt-get -y install dnsutils iputils-ping telnet netcat sysstat vim
wget https://github.com/bestmethod/tcconfig-ubuntu-venv/archive/tc.tar.gz || exit 1
tar -zxvf tc.tar.gz || exit 1
mv /tcconfig-ubuntu-venv-tc /tcconfig || exit 1
mv /tcconfig/lib/* /tcconfig/lib/python$(python3 --version |egrep -o '[0-9]+\.[0-9]+')
cd /root && tar -zxf installer.tgz || exit 1
cd aerospike-server-* ; ./asinstall || exit 1
cat <<'EOF' > /usr/local/bin/early.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
cat <<'EOF' > /usr/local/bin/late.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
chmod 755 /usr/local/bin/early.sh
chmod 755 /usr/local/bin/late.sh
apt-get -y install libldap-2.4-2 ; apt -y --fix-broken install ; apt-get -y install libldap-2.4-2
cat <<'EOF' > /etc/apt/apt.conf.d/local
Dpkg::Options {
	"--force-confdef";
	"--force-confold";
}
EOF
cat <<'EOF' > /etc/dpkg/dpkg.cfg.d/local
force-confdef
force-confold
EOF
cat <<'EOF' > /etc/init.d/aerospike
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
		/bin/bash /usr/local/bin/early.sh
		ulimit -n 1048576
		start_daemon -p $PIDFILE $DAEMON $EXTRA_OPTS
		log_end_msg $?
		;;
stop)   log_daemon_msg "Stopping aerospike" "aerospike"
		pkill asd
		RETVAL=$?
		if [ $RETVAL -ne 0 ]; then pkill -9 asd; fi 
		/bin/bash /usr/local/bin/late.sh
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
`

	aerospikeInstallScript["docker:ubuntu:20.04"] = aerospikeInstallScript["docker:ubuntu:22.04"]
	aerospikeInstallScript["docker:ubuntu:18.04"] = aerospikeInstallScript["docker:ubuntu:22.04"]

	aerospikeInstallScript["docker:centos:7"] = `#!/bin/bash
set -o xtrace
yum -y update || exit 1
yum -y install iptables wget tcpdump which redhat-lsb-core initscripts binutils iproute iproute-tc libcurl-openssl-devel || exit 1
yum -y install dnsutils || yum -y install bind-utils
yum -y install python
yum -y install initscripts || exit 1
yum -y install redhat-lsb || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
yum -y install centos-release-scl ; yum install -y rh-python36 ; yum -y install python38 || yum -y install python36
cd /root && tar -zxvf installer.tgz || exit 1
cd aerospike-server-* ; ./asinstall || exit 1
cat <<'EOF' > /usr/local/bin/early.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
cat <<'EOF' > /usr/local/bin/late.sh
#!/bin/bash
ls / >/dev/null 2>&1
EOF
chmod 755 /usr/local/bin/early.sh
chmod 755 /usr/local/bin/late.sh
cat <<'EOF' > /etc/init.d/aerospike
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
		/bin/bash /usr/local/bin/early.sh
		start_daemon -p $PIDFILE $DAEMON $EXTRA_OPTS
		log_success_msg $?
		;;
stop)   log_success_msg "Stopping aerospike" "aerospike"
		pkill asd
		sleep 1
		if [ $? -ne 0 ]; then pkill -9 asd; fi
		/bin/bash /usr/local/bin/late.sh
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
`

	aerospikeInstallScript["docker:centos:8"] = strings.Replace(strings.Replace(aerospikeInstallScript["docker:centos:7"], "#!/bin/bash", "#!/bin/bash\ndnf --disablerepo '*' --enablerepo=extras swap centos-linux-repos centos-stream-repos -y && dnf distro-sync -y || exit 1", 1), "libcurl-openssl-devel", "libcurl-devel", 1)
	aerospikeInstallScript["docker:amazon:2"] = aerospikeInstallScript["docker:centos:7"]

	aerospikeInstallScript["docker:debian:11"] = aerospikeInstallScript["docker:ubuntu:22.04"]
	aerospikeInstallScript["docker:debian:10"] = aerospikeInstallScript["docker:debian:11"]
	aerospikeInstallScript["docker:debian:9"] = aerospikeInstallScript["docker:debian:11"]
	aerospikeInstallScript["docker:debian:8"] = aerospikeInstallScript["docker:debian:11"]

	aerospikeInstallScript["aws:debian:11"] = aerospikeInstallScript["aws:ubuntu:22.04"]
	aerospikeInstallScript["aws:debian:10"] = aerospikeInstallScript["aws:debian:11"]
	aerospikeInstallScript["aws:debian:9"] = aerospikeInstallScript["aws:debian:11"]
	aerospikeInstallScript["aws:debian:8"] = aerospikeInstallScript["aws:debian:11"]

}
