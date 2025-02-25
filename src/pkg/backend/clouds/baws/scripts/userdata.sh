#!/bin/bash

REBOOT=0
NEW_KEY="%s"

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
if [ $REBOOT -eq 1 ]; then
    echo "Restarting sshd"
    if ! systemctl restart sshd; then
        echo "Failed to restart sshd, trying ssh"
        if ! systemctl restart ssh; then
            echo "Failed to restart ssh, exiting"
            exit 1
        fi
    fi
fi

if [ ! -f /root/.ssh/authorized_keys ]; then
    echo "Creating authorized_keys for root"
    mkdir -p /root/.ssh
    chmod 700 /root/.ssh
    touch /root/.ssh/authorized_keys
    chmod 600 /root/.ssh/authorized_keys
fi

if ! grep -q "$NEW_KEY" /root/.ssh/authorized_keys; then
    echo "Adding new key to root's authorized_keys"
    echo "$NEW_KEY" >> /root/.ssh/authorized_keys
fi
