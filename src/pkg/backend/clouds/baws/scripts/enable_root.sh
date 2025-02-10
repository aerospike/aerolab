#!/bin/bash

REBOOT=0

# check if PermitRootLogin is set to prohibit-password
echo "Checking if PermitRootLogin is set to prohibit-password"
grep -E -q '^PermitRootLogin prohibit-password' /etc/ssh/sshd_config
if [ $? -ne 0 ]; then
    grep -E -q '^PermitRootLogin ' /etc/ssh/sshd_config
    if [ $? -ne 0 ]; then
        echo "PermitRootLogin is not set at all, setting it"
        echo "PermitRootLogin prohibit-password" |sudo tee -a /etc/ssh/sshd_config >/dev/null || exit 1
    else
        echo "PermitRootLogin is not set to prohibit-password, setting it"
        sudo sed -i 's/^PermitRootLogin .*/PermitRootLogin prohibit-password/' /etc/ssh/sshd_config || exit 1
    fi
    REBOOT=1
else
    echo "PermitRootLogin is already set to prohibit-password"
fi

# check if env variable passing is enabled in sshd_config, enable if it is not
echo "Checking if AcceptEnv is set to AEROLAB_*"
grep -E -q '^AcceptEnv AEROLAB_*' /etc/ssh/sshd_config
if [ $? -ne 0 ]; then
    echo "AcceptEnv is not set, setting it"
    echo "AcceptEnv AEROLAB_*" |sudo tee -a /etc/ssh/sshd_config >/dev/null || exit 1
    REBOOT=1
else
    echo "AcceptEnv is already set to AEROLAB_*"
fi

# if REBOOT is set, restart sshd
if [ $REBOOT -eq 1 ]; then
    echo "Restarting sshd"
    sudo systemctl restart sshd || exit 1
fi

# copy current user's authorized_keys to root
set -e
echo "Copying authorized_keys to root"
sudo mkdir -p /root/.ssh
sudo chown root:root /root/.ssh
sudo chmod 700 /root/.ssh
sudo touch /root/.ssh/authorized_keys
set +e

cat ~/.ssh/authorized_keys |while read line; do
    sudo grep -q "$line" /root/.ssh/authorized_keys
    if [ $? -ne 0 ]; then
        echo "$line" |sudo tee -a /root/.ssh/authorized_keys >/dev/null || exit 1
    fi
done

set -e
sudo chown root:root /root/.ssh/authorized_keys
sudo chmod 600 /root/.ssh/authorized_keys
echo "Done"
