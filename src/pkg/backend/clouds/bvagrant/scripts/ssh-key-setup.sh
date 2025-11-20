#!/bin/bash
set -e

# Setup SSH key for root - copy from vagrant user first
mkdir -p /root/.ssh
if [ -f /home/vagrant/.ssh/authorized_keys ]; then
    cat /home/vagrant/.ssh/authorized_keys >> /root/.ssh/authorized_keys
fi
echo '{{PUBLIC_KEY}}' >> /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys
chmod 700 /root/.ssh

# Setup SSH key for vagrant user (aerolab managed key)
mkdir -p /home/vagrant/.ssh
echo '{{PUBLIC_KEY}}' >> /home/vagrant/.ssh/authorized_keys
chown -R vagrant:vagrant /home/vagrant/.ssh
chmod 600 /home/vagrant/.ssh/authorized_keys
chmod 700 /home/vagrant/.ssh

