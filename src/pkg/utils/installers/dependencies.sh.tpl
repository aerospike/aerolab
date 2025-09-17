#!/bin/bash
# shellcheck disable=SC1083
DEBUG="{{.Debug}}"

if [ "$DEBUG" == "true" ]; then
    set -x
fi

# set -e # die on any error
# set -u # if variable is not set, error
# set -o pipefail # die if any command in a pipeline fails, not just last one

# if apt - disable unattended-upgrades, and handle conflicts for configuration files
if command -v apt &> /dev/null; then
# unattended upgrades
export DEBIAN_FRONTEND=noninteractive
apt-get update || true
systemctl stop unattended-upgrades || true
pkill --signal SIGKILL unattended-upgrades || true
systemctl disable unattended-upgrades || true
apt-get -y -f install || true
apt-get -y purge unattended-upgrades || true
# restart if we have to
sed -i.bak "/#\$nrconf{restart} = .*/s/.*/\$nrconf{restart} = 'a';/" /etc/needrestart/needrestart.conf || true
# conflict handling for configuration files
cat <<'EOF' > /etc/apt/apt.conf.d/local || true
Dpkg::Options {
	"--force-confdef";
	"--force-confold";
}
EOF
cat <<'EOF' > /etc/dpkg/dpkg.cfg.d/local || true
force-confdef
force-confold
EOF
fi

# if yum - disable sshd-keygen cloud init
if command -v yum &> /dev/null; then
    rm -f /etc/systemd/system/sshd-keygen\@.service.d/disable-sshd-keygen-if-cloud-init-active.conf
    systemctl daemon-reload
fi

# check dependencies
# names of packages to install the dependencies, so if dependency 0 "curl" is missing, it will install package 0 "curl"
DEPS=({{ range .Required.Dependencies }}"{{ .Command }}" {{ end }})
PACKAGES=({{ range .Required.Dependencies }}"{{ .Package }}" {{ end }})
TO_INSTALL=({{ range .Required.Packages }}"{{ . }}" {{ end }})
for i in "${!DEPS[@]}"; do
    dep="${DEPS[$i]}"
    if ! command -v "$dep" &> /dev/null; then
        echo "Could not find $dep, adding to install list"
        TO_INSTALL+=("${PACKAGES[$i]}")
    fi
done

DEPS_OPTIONAL=({{ range .Optional.Dependencies }}"{{ .Command }}" {{ end }})
PACKAGES_OPTIONAL=({{ range .Optional.Dependencies }}"{{ .Package }}" {{ end }})
TO_INSTALL_OPTIONAL=({{ range .Optional.Packages }}"{{ . }}" {{ end }})
for i in "${!DEPS_OPTIONAL[@]}"; do
    dep="${DEPS_OPTIONAL[$i]}"
    if ! command -v "$dep" &> /dev/null; then
        echo "Could not find $dep, adding to install list"
        TO_INSTALL_OPTIONAL+=("${PACKAGES_OPTIONAL[$i]}")
    fi
done

# install dependencies
if [ ${#TO_INSTALL[@]} -gt 0 ]; then
    # patch yum if running centos:stream8
    if [ -f /etc/redhat-release ] && grep -q "CentOS Stream release 8" /etc/redhat-release; then
        echo "Patching yum for centos:stream8"
        sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
        sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
        sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
    fi
    echo "Installing dependencies: ${TO_INSTALL[*]}"
    if command -v apt &> /dev/null; then
        if [ ! -e /etc/localtime ]; then ln -fs /usr/share/zoneinfo/UTC /etc/localtime; fi
        set -e
        apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y "${TO_INSTALL[@]}"
        set +e
    elif command -v dnf &> /dev/null; then
        set -e
        dnf install -y "${TO_INSTALL[@]}"
        set +e
    elif command -v yum &> /dev/null; then
        set -e
        yum install -y "${TO_INSTALL[@]}"
        set +e
    else
        echo "Could not find package manager to install dependencies"
        exit 1
    fi
fi

# install optional dependencies
if [ ${#TO_INSTALL_OPTIONAL[@]} -gt 0 ]; then
    # patch yum if running centos:stream8
    if [ -f /etc/redhat-release ] && grep -q "CentOS Stream release 8" /etc/redhat-release; then
        echo "Patching yum for centos:stream8"
        sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
        sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
        sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
    fi
    echo "Installing dependencies: ${TO_INSTALL_OPTIONAL[*]}"
    if command -v apt &> /dev/null; then
        if [ ! -e /etc/localtime ]; then ln -fs /usr/share/zoneinfo/UTC /etc/localtime; fi
        apt-get update || true
        for pkg in "${TO_INSTALL_OPTIONAL[@]}"; do
            DEBIAN_FRONTEND=noninteractive apt-get install -y "$pkg" || true
        done
    elif command -v dnf &> /dev/null; then
        for pkg in "${TO_INSTALL_OPTIONAL[@]}"; do
            dnf install -y "$pkg" || true
        done
    elif command -v yum &> /dev/null; then
        for pkg in "${TO_INSTALL_OPTIONAL[@]}"; do
            yum install -y "$pkg" || true
        done
    else
        echo "Could not find package manager to install optional dependencies"
    fi
fi
