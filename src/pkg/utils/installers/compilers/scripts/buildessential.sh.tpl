#!/bin/bash

# this installs yum groupinstall 'Development Tools' or apt-get install build-essential git
# this provides gcc, g++, make, git, and optionally java (openjdk-8 will be auto-installed on yum-based systems, openjdk-21 will be installed on request on ubuntu/debian)

INSTALL_JAVA_APT="{{.InstallJavaApt}}" # this should be the package name for apt, ex: openjdk-21-jre-headless
INSTALL_JAVA_YUM="{{.InstallJavaYum}}" # this should be the package name for yum, ex: java-21-openjdk-headless

if command -v apt &> /dev/null; then
    apt-get update
    if [ ! -f /etc/localtime ]; then
        ln -fs /usr/share/zoneinfo/America/Los_Angeles /etc/localtime
    fi
    DEBIAN_FRONTEND=noninteractive apt-get install -y build-essential git $INSTALL_JAVA_APT || exit 1
elif command -v yum &> /dev/null; then
    if [ -f /etc/redhat-release ] && grep -q "CentOS Stream release 8" /etc/redhat-release; then
        echo "Patching yum for centos:stream8"
        sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
        sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
        sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
    fi
    yum groupinstall -y 'Development Tools' || exit 1
    if [ "$INSTALL_JAVA_YUM" != "" ]; then
        yum install -y $INSTALL_JAVA_YUM || exit 1
    fi
else
    echo "Neither apt nor yum found. Cannot install build-essential."
    exit 1
fi
