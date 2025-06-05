#!/bin/bash
# shellcheck disable=SC1083
DEBUG="{{.Debug}}"

if [ "$DEBUG" == "true" ]; then
    set -x
fi

# set -e # die on any error
# set -u # if variable is not set, error
# set -o pipefail # die if any command in a pipeline fails, not just last one

# check dependencies
# names of packages to install the dependencies, so if dependency 0 "curl" is missing, it will install package 0 "curl"
DEPS=({{ range .Dependencies }}"{{ . }}" {{ end }})
PACKAGES=({{ range .Packages }}"{{ . }}" {{ end }})
TO_INSTALL=()
for i in "${!DEPS[@]}"; do
    dep="${DEPS[$i]}"
    if ! command -v "$dep" &> /dev/null; then
        echo "Could not find $dep, adding to install list"
        TO_INSTALL+=("${PACKAGES[$i]}")
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
