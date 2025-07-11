#!/bin/bash

# shellcheck disable=SC1083
REQUIRED_PIP_PACKAGES=({{range .RequiredPipPackages}}"{{.}}" {{end}})
OPTIONAL_PIP_PACKAGES=({{range .OptionalPipPackages}}"{{.}}" {{end}})
EXTRA_APT_PACKAGES=({{range .ExtraAptPackages}}"{{.}}" {{end}})
EXTRA_YUM_PACKAGES=({{range .ExtraYumPackages}}"{{.}}" {{end}})

RAN_APT_UPDATE=false

if ! command -v python3 &> /dev/null; then
    if command -v apt &> /dev/null; then
        if [ ! -f /etc/localtime ]; then
            ln -fs /usr/share/zoneinfo/America/Los_Angeles /etc/localtime
        fi
        if [ "$RAN_APT_UPDATE" = false ]; then
            apt-get update
            RAN_APT_UPDATE=true
        fi
        DEBIAN_FRONTEND=noninteractive apt-get install -y python3 python3-pip || exit 1
    elif command -v yum &> /dev/null; then
        if [ -f /etc/redhat-release ] && grep -q "CentOS Stream release 8" /etc/redhat-release; then
            echo "Patching yum for centos:stream8"
            sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
            sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
            sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
        fi
        yum install -y python3 python3-pip || exit 1
    else
        echo "Neither apt nor yum found. Cannot install python3."
        exit 1
    fi
fi

if ! command -v pip3 &> /dev/null; then
    if command -v apt &> /dev/null; then
        if [ ! -f /etc/localtime ]; then
            ln -fs /usr/share/zoneinfo/America/Los_Angeles /etc/localtime
        fi
        if [ "$RAN_APT_UPDATE" = false ]; then
            apt-get update
            RAN_APT_UPDATE=true
        fi
        DEBIAN_FRONTEND=noninteractive apt-get install -y python3-pip || exit 1
    elif command -v yum &> /dev/null; then
        if [ -f /etc/redhat-release ] && grep -q "CentOS Stream release 8" /etc/redhat-release; then
            echo "Patching yum for centos:stream8"
            sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
            sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
            sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
        fi
        yum install -y python3-pip || exit 1
    else
        echo "Neither apt nor yum found. Cannot install pip3."
        exit 1
    fi
fi

if command -v apt &> /dev/null; then
    if [ ! -f /etc/localtime ]; then
        ln -fs /usr/share/zoneinfo/America/Los_Angeles /etc/localtime
    fi
    if [ "$RAN_APT_UPDATE" = false ]; then
        apt-get update
        RAN_APT_UPDATE=true
    fi
    DEBIAN_FRONTEND=noninteractive apt-get install -y "${EXTRA_APT_PACKAGES[@]}" || exit 1
elif command -v yum &> /dev/null; then
    if [ -f /etc/redhat-release ] && grep -q "CentOS Stream release 8" /etc/redhat-release; then
        echo "Patching yum for centos:stream8"
        sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
        sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
        sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
    fi
    yum install -y "${EXTRA_YUM_PACKAGES[@]}" || exit 1
else
    echo "Neither apt nor yum found. Cannot install extra packages."
    exit 1
fi

if [ ! -f ~/.config/pip/pip.conf ]; then
    mkdir -p ~/.config/pip
    cat <<'EOF' > ~/.config/pip/pip.conf
[global]
ignore-installed = true
break-system-packages = true
EOF
fi

python3 -m pip install --upgrade pip wheel setuptools || exit 1

python3 -m pip install --upgrade "${REQUIRED_PIP_PACKAGES[@]}" || exit 1

for pkg in "${OPTIONAL_PIP_PACKAGES[@]}"; do
    python3 -m pip install --upgrade "$pkg" || true
done
