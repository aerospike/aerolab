package main

var aerospikeInstallScript = make(map[string]string) //docker:centos:7 = script; or aws:ubuntu:20.04 = script

func init() {

	aerospikeInstallScript["aws:ubuntu:22.04"] = `#!/bin/bash
systemctl stop unattended-upgrades || echo "1:OK"
pkill --signal SIGKILL unattended-upgrades || echo "2:OK"
systemctl disable unattended-upgrades || echo "3:OK"
apt-get -y -f install || echo "4:OK"
apt-get -y purge unattended-upgrades || echo "5:OK"
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
grep DISTRIB_RELEASE=24.04 /etc/lsb-release
if [ $? -ne 0 ]; then apt-get -y install python3-distutils || apt-get update && apt-get -y install python3-distutils || exit 1; fi
apt-get -y install libcurl4 || apt-get update && apt-get -y install libcurl4 || exit 1
apt-get -y install ldap-utils || apt-get update && apt-get -y install ldap-utils || exit 1
apt-get -y install python3-setuptools || apt-get update && apt-get -y install python3-setuptools || exit 1
apt-get -y install python
apt-get -y install nano lnav
apt-get -y install iptables wget dnsutils tcpdump net-tools vim binutils iproute2 python3 libcurl4-openssl-dev less || apt-get update && apt-get -y install iptables wget dnsutils tcpdump net-tools vim binutils iproute2 python3 libcurl4-openssl-dev less || exit 1
apt-get -y install dnsutils iputils-ping telnet netcat sysstat vim
########### tcconfig
VERSION_ID=$(grep -Po '(?<=VERSION_ID=")[0-9\.]+' /etc/os-release)
CPUVER=amd64
[ "$(uname -p)" = "arm64" ] && CPUVER=arm64
[ "$(uname -p)" = "aarch64" ] && CPUVER=arm64
tcfn="tcconfig-${VERSION_ID}-${CPUVER}.deb"
wget https://github.com/rglonek/tcconfig-builds/releases/download/v0.29.1-1/${tcfn} || echo "no net-loss-delay support"
dpkg --force-architecture -i ${tcfn} || echo "no net-loss-delay support"
########## tcconfig end
cd /root && tar -zxf installer.tgz || exit 1
cd aerospike-server-* ; ./asinstall || exit 1
apt-get -y install libldap-2.4-2 ; apt -y --fix-broken install ; apt-get -y install libldap-2.4-2 || apt-get -y install libldap-common
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

	aerospikeInstallScript["aws:ubuntu:24.04"] = aerospikeInstallScript["aws:ubuntu:22.04"]
	aerospikeInstallScript["aws:ubuntu:20.04"] = aerospikeInstallScript["aws:ubuntu:22.04"]
	aerospikeInstallScript["aws:ubuntu:18.04"] = aerospikeInstallScript["aws:ubuntu:22.04"]
	aerospikeInstallScript["gcp:ubuntu:24.04"] = aerospikeInstallScript["aws:ubuntu:24.04"]
	aerospikeInstallScript["gcp:ubuntu:22.04"] = aerospikeInstallScript["aws:ubuntu:22.04"]
	aerospikeInstallScript["gcp:ubuntu:20.04"] = aerospikeInstallScript["aws:ubuntu:20.04"]
	aerospikeInstallScript["gcp:ubuntu:18.04"] = aerospikeInstallScript["aws:ubuntu:18.04"]

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
sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
yum -y update || exit 1
yum -y install centos-release-scl
sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
yum -y install iptables wget tcpdump which redhat-lsb-core initscripts binutils iproute iproute-tc libcurl-openssl-devel || exit 1
yum -y install dnsutils || yum -y install bind-utils
yum -y install python
yum -y install nano lnav
yum -y install initscripts || exit 1
yum -y install redhat-lsb || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
yum install -y rh-python36 ; yum -y install python38 || yum -y install python36 || echo "python36 skip"
########### tcconfig
VERSION_ID=$(grep -Po '(?<=VERSION_ID=")[0-9]+' /etc/os-release)
CPUVER=amd64
[ "$(uname -p)" = "arm64" ] && CPUVER=arm64
[ "$(uname -p)" = "aarch64" ] && CPUVER=arm64
tcfn="tcconfig-centos-${VERSION_ID}-${CPUVER}.tgz"
wget https://github.com/rglonek/tcconfig-builds/releases/download/v0.29.1-1/${tcfn} || echo "no net-loss-delay support"
tar -C /usr/local/bin -zxvf ${tcfn} || echo "no net-loss-delay support"
chmod 755 /usr/local/bin/tc* || echo "no net-loss-delay support"
########## tcconfig end
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
sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
yum -y update || exit 1
yum -y install iptables wget tcpdump which redhat-lsb-core initscripts binutils iproute iproute-tc libcurl-devel || exit 1
yum -y install dnsutils || yum -y install bind-utils
yum -y install python
yum -y install nano lnav
yum -y install initscripts || exit 1
yum -y install redhat-lsb || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
yum -y install centos-release-scl ; yum install -y rh-python36 ; yum -y install python38 || yum -y install python36
########### tcconfig
VERSION_ID=$(grep -Po '(?<=VERSION_ID=")[0-9]+' /etc/os-release)
CPUVER=amd64
[ "$(uname -p)" = "arm64" ] && CPUVER=arm64
[ "$(uname -p)" = "aarch64" ] && CPUVER=arm64
tcfn="tcconfig-centos-stream${VERSION_ID}-${CPUVER}.tgz"
wget https://github.com/rglonek/tcconfig-builds/releases/download/v0.29.1-1/${tcfn} || echo "no net-loss-delay support"
tar -C /usr/local/bin -zxvf ${tcfn} || echo "no net-loss-delay support"
chmod 755 /usr/local/bin/tc* || echo "no net-loss-delay support"
########## tcconfig end
cd /root && tar -zxvf installer.tgz || exit 1
cd aerospike-server-* ; ./asinstall || exit 1
rm -f /etc/systemd/system/sshd-keygen\@.service.d/disable-sshd-keygen-if-cloud-init-active.conf
systemctl daemon-reload
`
	//systemctl enable --now cockpit.socket; echo b0bTheBuilder |passwd --stdin root;  echo b0bTheBuilder |passwd --stdin centos
	aerospikeInstallScript["aws:centos:9"] = `#!/bin/bash
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
yum -y install iptables wget tcpdump which initscripts binutils iproute iproute-tc libcurl-devel || exit 1
yum -y install dnsutils || yum -y install bind-utils
yum -y install python
yum -y install nano lnav
yum -y install initscripts || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
########### tcconfig
VERSION_ID=$(grep -Po '(?<=VERSION_ID=")[0-9]+' /etc/os-release)
[ "${VERSION_ID}" = "2023" ] && VERSION_ID=9
CPUVER=amd64
[ "$(uname -p)" = "arm64" ] && CPUVER=arm64
[ "$(uname -p)" = "aarch64" ] && CPUVER=arm64
tcfn="tcconfig-centos-stream${VERSION_ID}-${CPUVER}.tgz"
wget https://github.com/rglonek/tcconfig-builds/releases/download/v0.29.1-1/${tcfn} || echo "no net-loss-delay support"
tar -C /usr/local/bin -zxvf ${tcfn} || echo "no net-loss-delay support"
chmod 755 /usr/local/bin/tc* || echo "no net-loss-delay support"
########## tcconfig end
cd /root && tar -zxvf installer.tgz || exit 1
cd aerospike-server-* ; ./asinstall || exit 1
rm -f /etc/systemd/system/sshd-keygen\@.service.d/disable-sshd-keygen-if-cloud-init-active.conf || echo "Not there"
systemctl daemon-reload
`

	aerospikeInstallScript["gcp:centos:7"] = aerospikeInstallScript["aws:centos:7"]
	aerospikeInstallScript["gcp:centos:8"] = aerospikeInstallScript["aws:centos:8"]
	aerospikeInstallScript["gcp:centos:9"] = aerospikeInstallScript["aws:centos:9"]

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
yum -y install nano lnav
yum -y install initscripts || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
yum -y install centos-release-scl ; yum install -y rh-python36 ; yum -y install python38 || yum -y install python36 || echo "python36 skip"
########### tcconfig
CPUVER=amd64
[ "$(uname -p)" = "arm64" ] && CPUVER=arm64
[ "$(uname -p)" = "aarch64" ] && CPUVER=arm64
tcfn="tcconfig-centos-7-${CPUVER}.tgz"
wget https://github.com/rglonek/tcconfig-builds/releases/download/v0.29.1-1/${tcfn} || echo "no net-loss-delay support"
tar -C /usr/local/bin -zxvf ${tcfn} || echo "no net-loss-delay support"
chmod 755 /usr/local/bin/tc* || echo "no net-loss-delay support"
########## tcconfig end
cd /root && tar -zxvf installer.tgz || exit 1
cd aerospike-server-* ; ./asinstall || exit 1
`

	aerospikeInstallScript["aws:amazon:2023"] = aerospikeInstallScript["aws:centos:9"]
	aerospikeInstallScript["gcp:amazon:2"] = aerospikeInstallScript["aws:centos:7"]

	aerospikeInstallScript["docker:ubuntu:22.04"] = `#!/bin/bash
export DEBIAN_FRONTEND=noninteractive
apt-get update || exit 1
grep DISTRIB_RELEASE=24.04 /etc/lsb-release
if [ $? -ne 0 ]; then apt-get -y install python3-distutils || exit 1; fi
apt-get -y install libcurl4 || exit 1
apt-get -y install ldap-utils || exit 1
apt-get -y install python3-setuptools || exit 1
apt-get -y install python
apt-get -y install nano lnav
apt-get -y install iptables wget dnsutils tcpdump net-tools vim binutils iproute2 python3 libcurl4-openssl-dev less || exit 1
apt-get -y install dnsutils iputils-ping telnet netcat sysstat vim
########### tcconfig
VERSION_ID=$(grep -Po '(?<=VERSION_ID=")[0-9\.]+' /etc/os-release)
CPUVER=amd64
[ "$(uname -p)" = "arm64" ] && CPUVER=arm64
[ "$(uname -p)" = "aarch64" ] && CPUVER=arm64
tcfn="tcconfig-${VERSION_ID}-${CPUVER}.deb"
wget https://github.com/rglonek/tcconfig-builds/releases/download/v0.29.1-1/${tcfn} || echo "no net-loss-delay support"
dpkg --force-architecture -i ${tcfn} || echo "no net-loss-delay support"
########## tcconfig end
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
apt-get -y install libldap-2.4-2 ; apt -y --fix-broken install ; apt-get -y install libldap-2.4-2 || apt-get -y install libldap-common
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
		ulimit -n 1048576 || echo "ulimit not set"
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

	aerospikeInstallScript["docker:ubuntu:24.04"] = aerospikeInstallScript["docker:ubuntu:22.04"]
	aerospikeInstallScript["docker:ubuntu:20.04"] = aerospikeInstallScript["docker:ubuntu:22.04"]
	aerospikeInstallScript["docker:ubuntu:18.04"] = aerospikeInstallScript["docker:ubuntu:22.04"]

	aerospikeInstallScript["docker:centos:7"] = `#!/bin/bash
set -o xtrace
sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
yum -y update || exit 1
yum -y install centos-release-scl
sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
yum -y install iptables wget tcpdump which redhat-lsb-core initscripts binutils iproute iproute-tc libcurl-openssl-devel || exit 1
yum -y install dnsutils || yum -y install bind-utils
yum -y install python
yum -y install nano lnav
yum -y install initscripts || exit 1
yum -y install redhat-lsb || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
yum install -y rh-python36 ; yum -y install python38 || yum -y install python36 || echo "python36 skip"
########### tcconfig
VERSION_ID=$(grep -Po '(?<=VERSION_ID=")[0-9]+' /etc/os-release)
CPUVER=amd64
[ "$(uname -p)" = "arm64" ] && CPUVER=arm64
[ "$(uname -p)" = "aarch64" ] && CPUVER=arm64
tcfn="tcconfig-centos-${VERSION_ID}-${CPUVER}.tgz"
wget https://github.com/rglonek/tcconfig-builds/releases/download/v0.29.1-1/${tcfn} || echo "no net-loss-delay support"
tar -C /usr/local/bin -zxvf ${tcfn} || echo "no net-loss-delay support"
chmod 755 /usr/local/bin/tc* || echo "no net-loss-delay support"
########## tcconfig end
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

	aerospikeInstallScript["docker:centos:8"] = `#!/bin/bash
set -o xtrace
sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
yum -y update || exit 1
yum -y install iptables wget tcpdump which redhat-lsb-core initscripts binutils iproute iproute-tc libcurl-devel || exit 1
yum -y install dnsutils || yum -y install bind-utils
yum -y install python
yum -y install nano lnav
yum -y install initscripts || exit 1
yum -y install redhat-lsb || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
yum -y install centos-release-scl ; yum install -y rh-python36 ; yum -y install python38 || yum -y install python36
########### tcconfig
VERSION_ID=$(grep -Po '(?<=VERSION_ID=")[0-9]+' /etc/os-release)
CPUVER=amd64
[ "$(uname -p)" = "arm64" ] && CPUVER=arm64
[ "$(uname -p)" = "aarch64" ] && CPUVER=arm64
tcfn="tcconfig-centos-stream${VERSION_ID}-${CPUVER}.tgz"
wget https://github.com/rglonek/tcconfig-builds/releases/download/v0.29.1-1/${tcfn} || echo "no net-loss-delay support"
tar -C /usr/local/bin -zxvf ${tcfn} || echo "no net-loss-delay support"
chmod 755 /usr/local/bin/tc* || echo "no net-loss-delay support"
########## tcconfig end
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
	aerospikeInstallScript["docker:amazon:2"] = aerospikeInstallScript["docker:centos:7"]

	aerospikeInstallScript["docker:debian:12"] = aerospikeInstallScript["docker:ubuntu:22.04"]
	aerospikeInstallScript["docker:debian:11"] = aerospikeInstallScript["docker:ubuntu:22.04"]
	aerospikeInstallScript["docker:debian:10"] = aerospikeInstallScript["docker:debian:11"]
	aerospikeInstallScript["docker:debian:9"] = aerospikeInstallScript["docker:debian:11"]
	aerospikeInstallScript["docker:debian:8"] = aerospikeInstallScript["docker:debian:11"]

	aerospikeInstallScript["aws:debian:12"] = aerospikeInstallScript["aws:ubuntu:22.04"]
	aerospikeInstallScript["aws:debian:11"] = aerospikeInstallScript["aws:ubuntu:22.04"]
	aerospikeInstallScript["aws:debian:10"] = aerospikeInstallScript["aws:debian:11"]
	aerospikeInstallScript["aws:debian:9"] = aerospikeInstallScript["aws:debian:11"]
	aerospikeInstallScript["aws:debian:8"] = aerospikeInstallScript["aws:debian:11"]

	aerospikeInstallScript["gcp:debian:12"] = aerospikeInstallScript["aws:ubuntu:22.04"]
	aerospikeInstallScript["gcp:debian:11"] = aerospikeInstallScript["aws:ubuntu:22.04"]
	aerospikeInstallScript["gcp:debian:10"] = aerospikeInstallScript["aws:debian:11"]
	aerospikeInstallScript["gcp:debian:9"] = aerospikeInstallScript["aws:debian:11"]
	aerospikeInstallScript["gcp:debian:8"] = aerospikeInstallScript["aws:debian:11"]

	aerospikeInstallScript["docker:centos:9"] = `#!/bin/bash
set -o xtrace
yum -y update || exit 1
yum -y install iptables wget tcpdump which initscripts binutils iproute iproute-tc libcurl-devel || exit 1
yum -y install dnsutils || yum -y install bind-utils
yum -y install python
yum -y install nano lnav
yum -y install initscripts || exit 1
yum -y install telnet sysstat nc bind-utils iputils vim
########### tcconfig
VERSION_ID=$(grep -Po '(?<=VERSION_ID=")[0-9]+' /etc/os-release)
CPUVER=amd64
[ "$(uname -p)" = "arm64" ] && CPUVER=arm64
[ "$(uname -p)" = "aarch64" ] && CPUVER=arm64
tcfn="tcconfig-centos-stream${VERSION_ID}-${CPUVER}.tgz"
wget https://github.com/rglonek/tcconfig-builds/releases/download/v0.29.1-1/${tcfn} || echo "no net-loss-delay support"
tar -C /usr/local/bin -zxvf ${tcfn} || echo "no net-loss-delay support"
chmod 755 /usr/local/bin/tc* || echo "no net-loss-delay support"
########## tcconfig end
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

case "$1" in
start)  echo "Starting aerospike"
		/bin/bash /usr/local/bin/early.sh
		${DAEMON}
		[ $? -eq 0 ] && echo "OK" || echo "ERROR"
		;;
stop)   echo "Stopping aeropiske"
		pkill asd
		sleep 1
		if [ $? -ne 0 ]; then pkill -9 asd; fi
		/bin/bash /usr/local/bin/late.sh
		[ $? -eq 0 ] && echo "OK" || echo "ERROR"
		;;
restart) echo "Restarting aerospike"
		$0 stop
		$0 start
		;;
coldstart)  echo "Starting aerospike cold"
		$DAEMON --cold-start
		[ $? -eq 0 ] && echo "OK" || echo "ERROR"
		;;
status)
		pidof asd >/dev/null 2>&1
		[ $? -eq 0 ] && echo "Running" || echo "Stopped"
		;;
*)      echo "Usage: /etc/init.d/aerospike {start|stop|status|restart|coldstart}"
		exit 2
		;;
esac
exit 0
EOF
chmod 755 /etc/init.d/aerospike
`

	aerospikeInstallScript["gcp:rocky:8"] = aerospikeInstallScript["gcp:centos:8"]
	aerospikeInstallScript["gcp:rocky:9"] = aerospikeInstallScript["gcp:centos:9"]
	aerospikeInstallScript["aws:rocky:8"] = aerospikeInstallScript["aws:centos:8"]
	aerospikeInstallScript["aws:rocky:9"] = aerospikeInstallScript["aws:centos:9"]
	aerospikeInstallScript["docker:rocky:8"] = aerospikeInstallScript["docker:centos:8"]
	aerospikeInstallScript["docker:rocky:9"] = aerospikeInstallScript["docker:centos:9"]
}
