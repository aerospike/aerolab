#!/bin/bash

# backup
cp /etc/hosts /etc/hosts.backup

# remove all lines that end with # aerolab-managed
grep -v '# aerolab-managed' /etc/hosts > /etc/hosts.tmp

# append new lines
cat <<'EOF' >> /etc/hosts.tmp
# aerolab-managed list of hosts
%s
EOF

# replace original hosts file
mv /etc/hosts.tmp /etc/hosts
