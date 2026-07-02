#!/bin/bash

REBOOT=0
BUILDING=0
NEED_INIT_LINK=0
NEW_KEY=""
if [ -f /tmp/ssh_key ]; then
    NEW_KEY=$(cat /tmp/ssh_key)
fi
if [ -n "$SSH_PUBLIC_KEY" ]; then
    NEW_KEY="$SSH_PUBLIC_KEY"
fi

echo "=-=-=-= AEROLAB-INIT START =-=-=-="
# install self as a systemd service
if ! [ -f /etc/systemd/system/aerolab-init.service ]; then
    echo "Installing aerolab-init as a systemd service"
    mkdir -p /etc/systemd/system/multi-user.target.wants
    cat <<EOF > /etc/systemd/system/aerolab-init.service
[Unit]
Description=AEROLAB-INIT
After=network.target

[Service]
Type=oneshot
ExecStart=/bin/bash /opt/aerolab/scripts/userdata.sh

[Install]
WantedBy=multi-user.target
EOF
    NEED_INIT_LINK=1
else
    echo "aerolab-init service already installed"
fi

# if openssh not found, mark it for linking for autostart
if ! command -v sshd >/dev/null 2>&1; then
    BUILDING=1
fi

# patch yum if running centos:stream8
if [ -f /etc/redhat-release ] && grep -q "CentOS Stream release 8" /etc/redhat-release; then
    echo "Patching yum for centos:stream8"
    sed -i 's/mirror.centos.org/vault.centos.org/g' /etc/yum.repos.d/*.repo
    sed -i 's/^#.*baseurl=http/baseurl=http/g' /etc/yum.repos.d/*.repo
    sed -i 's/^mirrorlist=http/#mirrorlist=http/g' /etc/yum.repos.d/*.repo
fi

# get a list of dependencies that are missing
TO_INSTALL=""
if ! command -v jq >/dev/null 2>&1; then
    TO_INSTALL="$TO_INSTALL jq"
fi
if ! command -v curl >/dev/null 2>&1; then
    TO_INSTALL="$TO_INSTALL curl"
fi
if ! command -v sshd >/dev/null 2>&1; then
    TO_INSTALL="$TO_INSTALL openssh-server"
fi

# install the missing dependencies
if [ -n "$TO_INSTALL" ]; then
    echo "Installing dependencies: $TO_INSTALL"
    if command -v apt-get >/dev/null 2>&1; then
        if [ ! -e /etc/localtime ]; then ln -fs /usr/share/zoneinfo/UTC /etc/localtime; fi
        if ! apt-get update; then
            echo "ERROR: 'apt-get update' failed; check network egress and apt sources"
            exit 1
        fi
        if ! DEBIAN_FRONTEND=noninteractive apt-get install -y $TO_INSTALL; then
            echo "ERROR: failed to install dependencies via apt-get: $TO_INSTALL"
            exit 1
        fi
    elif command -v yum >/dev/null 2>&1; then
        if ! yum install -y $TO_INSTALL; then
            echo "ERROR: failed to install dependencies via yum: $TO_INSTALL"
            exit 1
        fi
    elif command -v dnf >/dev/null 2>&1; then
        if ! dnf install -y $TO_INSTALL; then
            echo "ERROR: failed to install dependencies via dnf: $TO_INSTALL"
            exit 1
        fi
    else
        echo "ERROR: no supported package manager (apt-get/yum/dnf) found to install: $TO_INSTALL"
        exit 1
    fi
    echo "Cleaning up to reduce image size..."
    if command -v apt-get >/dev/null 2>&1; then
        apt-get clean
        rm -rf /var/lib/apt/lists/*
        rm -rf /tmp/*
        rm -rf /var/tmp/*
        rm -rf /root/.cache/*
    elif command -v yum >/dev/null 2>&1 || command -v dnf >/dev/null 2>&1; then
        yum clean all || dnf clean all
        rm -rf /var/cache/yum
        rm -rf /tmp/*
        rm -rf /var/tmp/*
        rm -rf /root/.cache/*
    fi
fi

# get docker-systemd
if ! command -v init-docker-systemd >/dev/null 2>&1; then
    echo "Installing docker-systemd"
    ARCH="amd64"
    if [ "$(uname -m)" = "aarch64" ] || [ "$(uname -m)" = "arm64" ]; then
        ARCH="arm64"
    fi
    FN=systemd-$ARCH
    API_URL="https://api.github.com/repos/aerospike-community/docker-systemd/releases/latest"

    # Fetch the release metadata, keeping the HTTP status and body so we can
    # surface the real problem (network failure, GitHub API rate limiting,
    # missing asset, ...) instead of leaking a bare jq/curl exit code.
    API_RESP=$(curl -sS -w $'\n%{http_code}' "$API_URL" 2>/tmp/aerolab_curl_err)
    CURL_RC=$?
    if [ $CURL_RC -ne 0 ]; then
        echo "ERROR: could not reach GitHub API to locate docker-systemd."
        echo "       URL: $API_URL"
        echo "       curl exit code: $CURL_RC"
        echo "       curl stderr: $(cat /tmp/aerolab_curl_err 2>/dev/null)"
        rm -f /tmp/aerolab_curl_err
        exit 1
    fi
    rm -f /tmp/aerolab_curl_err
    HTTP_CODE=$(printf '%s' "$API_RESP" | tail -n1)
    API_BODY=$(printf '%s' "$API_RESP" | sed '$d')
    if [ "$HTTP_CODE" != "200" ]; then
        echo "ERROR: GitHub API returned HTTP $HTTP_CODE while locating docker-systemd."
        echo "       URL: $API_URL"
        echo "       Response: $API_BODY"
        if printf '%s' "$API_BODY" | grep -qi "rate limit"; then
            echo "       Hint: GitHub API rate limit reached (unauthenticated limit is 60 requests/hour per IP)."
            echo "             Wait and retry, run from an IP with higher quota, or pre-install"
            echo "             /usr/local/bin/init-docker-systemd so this lookup is skipped."
        fi
        exit 1
    fi

    DLURL=$(printf '%s' "$API_BODY" | jq -r ".assets[]? | select(.name == \"$FN\") | .browser_download_url" 2>/tmp/aerolab_jq_err)
    JQ_RC=$?
    if [ $JQ_RC -ne 0 ]; then
        echo "ERROR: failed to parse GitHub API response for docker-systemd (jq exit $JQ_RC)."
        echo "       jq stderr: $(cat /tmp/aerolab_jq_err 2>/dev/null)"
        echo "       Response: $API_BODY"
        rm -f /tmp/aerolab_jq_err
        exit 1
    fi
    rm -f /tmp/aerolab_jq_err
    if [ -z "$DLURL" ] || [ "$DLURL" = "null" ]; then
        echo "ERROR: no asset named '$FN' found in the latest docker-systemd release."
        echo "       Response: $API_BODY"
        exit 1
    fi

    echo "Downloading docker-systemd ($FN) from $DLURL"
    if ! curl -sS -L -o /usr/local/bin/init-docker-systemd "$DLURL" 2>/tmp/aerolab_curl_err; then
        echo "ERROR: failed to download docker-systemd from $DLURL"
        echo "       curl stderr: $(cat /tmp/aerolab_curl_err 2>/dev/null)"
        rm -f /tmp/aerolab_curl_err
        exit 1
    fi
    rm -f /tmp/aerolab_curl_err
    if ! chmod +x /usr/local/bin/init-docker-systemd; then
        echo "ERROR: failed to make /usr/local/bin/init-docker-systemd executable"
        exit 1
    fi
else
    echo "docker-systemd is already installed"
fi

# check if PermitRootLogin is set to prohibit-password
echo "Checking if PermitRootLogin is set to prohibit-password"
if ! grep -E -q '^PermitRootLogin prohibit-password' /etc/ssh/sshd_config; then
    if ! grep -E -q '^PermitRootLogin ' /etc/ssh/sshd_config; then
        echo "PermitRootLogin is not set at all, setting it"
        echo "PermitRootLogin prohibit-password" |tee -a /etc/ssh/sshd_config >/dev/null || exit 1
    else
        echo "PermitRootLogin is not set to prohibit-password, setting it"
        sed -i 's/^PermitRootLogin .*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config || exit 1
    fi
    REBOOT=1
else
    echo "PermitRootLogin is already set to prohibit-password"
fi

# check if env variable passing is enabled in sshd_config, enable if it is not
echo "Checking if AcceptEnv is set to AEROLAB_*"
if ! grep -E -q '^AcceptEnv AEROLAB_*' /etc/ssh/sshd_config; then
    echo "AcceptEnv is not set, setting it"
    echo "AcceptEnv AEROLAB_*" |tee -a /etc/ssh/sshd_config >/dev/null || exit 1
    REBOOT=1
else
    echo "AcceptEnv is already set to AEROLAB_*"
fi

# if REBOOT is set, restart sshd
if [ $BUILDING -eq 1 ]; then
    echo "Initial building, not rebooting. Enabling sshd for autostart"
    mkdir -p /run/sshd
    ssh-keygen -A || exit 1
    mkdir -p /etc/systemd/system/multi-user.target.wants
    [ -f /etc/systemd/system/sshd.service ] && ln -s /etc/systemd/system/sshd.service /etc/systemd/system/multi-user.target.wants/sshd.service
    [ -f /etc/systemd/system/ssh.service ] && ln -s /etc/systemd/system/ssh.service /etc/systemd/system/multi-user.target.wants/ssh.service
    [ -f /usr/lib/systemd/system/sshd.service ] && ln -s /usr/lib/systemd/system/sshd.service /etc/systemd/system/multi-user.target.wants/sshd.service
    [ -f /usr/lib/systemd/system/ssh.service ] && ln -s /usr/lib/systemd/system/ssh.service /etc/systemd/system/multi-user.target.wants/ssh.service
elif [ $REBOOT -eq 1 ]; then
    echo "Restarting sshd"
    if ! systemctl restart sshd; then
        echo "Failed to restart sshd, trying ssh"
        if ! systemctl restart ssh; then
            echo "Failed to restart ssh, exiting"
            exit 1
        fi
    fi
fi

if [ $NEED_INIT_LINK -eq 1 ]; then
    echo "Linking aerolab-init.service to multi-user.target.wants"
    ln -s /etc/systemd/system/aerolab-init.service /etc/systemd/system/multi-user.target.wants/aerolab-init.service || exit 1
fi

if [ ! -f /root/.ssh/authorized_keys ]; then
    echo "Creating authorized_keys for root"
    mkdir -p /root/.ssh
    chmod 700 /root/.ssh
    touch /root/.ssh/authorized_keys
    chmod 600 /root/.ssh/authorized_keys
else
    echo "authorized_keys file for root already exists"
fi

if [ -n "$NEW_KEY" ]; then
    if ! grep -q "$NEW_KEY" /root/.ssh/authorized_keys; then
        echo "Adding new key to root's authorized_keys"
        echo "$NEW_KEY" >> /root/.ssh/authorized_keys
    else
        echo "new key is already in root's authorized_keys"
    fi
    rm -f /tmp/ssh_key
else
    echo "No new ssh key provided"
fi

echo "Fixing RHEL crypto-policies if we need to"
rm -f /etc/crypto-policies/back-ends/opensshserver.config || true

echo "=-=-=-= AEROLAB-INIT END =-=-=-="
