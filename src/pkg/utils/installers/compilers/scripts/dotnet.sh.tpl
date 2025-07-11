#!/bin/bash
# shellcheck source=/dev/null

DOTNET_VERSION="{{.DotnetVersion}}" #ex: 9.0

# Check if Ubuntu or Debian
if [ -f /etc/os-release ]; then
    . /etc/os-release
    if [ "$ID" = "ubuntu" ]; then
        # Ubuntu specific commands
        if [ ! -f /etc/localtime ]; then
            ln -fs /usr/share/zoneinfo/America/Los_Angeles /etc/localtime
        fi
        apt update && DEBIAN_FRONTEND=noninteractive apt -y install software-properties-common
        add-apt-repository -y ppa:dotnet/backports
        DEBIAN_FRONTEND=noninteractive apt-get -y install dotnet-sdk-$DOTNET_VERSION aspnetcore-runtime-$DOTNET_VERSION dotnet-runtime-$DOTNET_VERSION
    elif [ "$ID" = "debian" ]; then
        # Debian specific commands
        if [ ! -f /etc/localtime ]; then
            ln -fs /usr/share/zoneinfo/America/Los_Angeles /etc/localtime
        fi
        apt update && DEBIAN_FRONTEND=noninteractive apt -y install curl
        curl -L -o /tmp/packages-microsoft-prod.deb https://packages.microsoft.com/config/debian/12/packages-microsoft-prod.deb
        dpkg -i /tmp/packages-microsoft-prod.deb
        apt-get update
        DEBIAN_FRONTEND=noninteractive apt-get -y install dotnet-sdk-$DOTNET_VERSION aspnetcore-runtime-$DOTNET_VERSION dotnet-runtime-$DOTNET_VERSION
    else
        # centos/rocky commands
        if grep -q "release 8" /etc/redhat-release; then
            # CentOS/Rocky 8 specific commands
            if [ -f /etc/redhat-release ] && grep -q "CentOS Stream release 8" /etc/redhat-release; then
                echo "Patching yum for centos:stream8"
                sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
                sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
                sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
            fi
            rpm -Uvh https://packages.microsoft.com/config/centos/8/packages-microsoft-prod.rpm
            yum -y install dotnet-sdk-$DOTNET_VERSION aspnetcore-runtime-$DOTNET_VERSION dotnet-runtime-$DOTNET_VERSION
        elif grep -q "release 9" /etc/redhat-release; then
            # CentOS/Rocky 9 specific commands
            dnf -y install dotnet-sdk-$DOTNET_VERSION aspnetcore-runtime-$DOTNET_VERSION dotnet-runtime-$DOTNET_VERSION
        else
            echo "Unsupported CentOS/Rocky version"
            exit 1
        fi
    fi
else
    echo "Cannot determine OS version"
    exit 1
fi
