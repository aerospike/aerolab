#!/bin/bash

### supported distros: rockylinux:9 rockylinux:8 ubuntu:24.04 ubuntu:22.04 ubuntu:20.04 debian:13 debian:12 debian:11 centos:stream9 centos:stream8 amazonlinux:2023 amazonlinux:2
### usage: ./efs_install.sh [fips]
### fips parameter will enable fips mode in efs utils; note: this requires that fips edition of openssh is installed

function install_efs_pre() {
    set -e
    curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh -s -- -y
    mkdir -p /opt/aerolab
    cd /opt/aerolab
}

function install_efs_clone() {
    git clone --depth 1 --branch "${1}" https://github.com/aws/efs-utils
    cd efs-utils
}

function install_efs_get_tag() {
    git ls-remote --tags --sort="v:refname" https://github.com/aws/efs-utils | tail -n 1 | sed 's/.*\///'
}

type -t yum >/dev/null 2>&1
if [ $? -eq 0 ]; then
    grep -q "Amazon Linux" /etc/os-release
    if [ $? -eq 0 ]; then
        # amazon linux
        set -e
        yum -y install amazon-efs-utils
    else
        # rpm
        type -t git >/dev/null 2>&1 || yum -y install git
        set -e
        #tag=$(install_efs_get_tag)
        tag=v2.3.3 # TODO: until we add all the huge dependencies to build v2.4.0
        set +e
        rpm -q amazon-efs-utils-${tag:1}
        if [ $? -ne 0 ]; then
            install_efs_pre
            yum -y install rpm-build make openssl-devel rust cargo systemd
            install_efs_clone ${tag}
            . "$HOME/.cargo/env"
            make rpm
            yum -y install build/amazon-efs-utils*rpm
        fi
    fi
else
    # deb
    if [ ! -e /etc/localtime ]; then ln -fs /usr/share/zoneinfo/UTC /etc/localtime; fi
    UPD=0
    type -t git >/dev/null 2>&1
    if [ $? -ne 0 ]; then
        set -e
        UPD=1
        apt-get update
        DEBIAN_FRONTEND=noninteractive apt-get -y install git
        set +e
    fi
    type -t curl >/dev/null 2>&1
    if [ $? -ne 0 ]; then
        set -e
        if [ ${UPD} -eq 0 ]; then UPD=1; apt-get update; fi
        DEBIAN_FRONTEND=noninteractive apt-get -y install curl
        set +e
    fi
    set -e
    #tag=$(install_efs_get_tag)
    tag=v2.3.3 # TODO: until we add all the huge dependencies to build v2.4.0
    set +e
    INST=0
    V=$(dpkg-query -W -f='${Version}\n' amazon-efs-utils)
    if [ $? -ne 0 ]; then
        INST=1
    else
        V=$(echo ${V} | sed 's/-.*//g')
        [ "$V" != "${tag:1}" ] && INST=1
    fi
    if [ $INST -ne 0 ]; then
        install_efs_pre
        if [ ${UPD} -eq 0 ]; then apt-get update; fi
        DEBIAN_FRONTEND=noninteractive apt-get -y install binutils rustc cargo pkg-config libssl-dev gettext
        install_efs_clone ${tag}
        . "$HOME/.cargo/env"
        ./build-deb.sh
        apt-get -y install ./build/amazon-efs-utils*deb
    fi
fi
if [ "$1" == "fips" ]; then
    sed -i "s/fips_mode_enabled = false/fips_mode_enabled = true/" /etc/amazon/efs/efs-utils.conf
fi
