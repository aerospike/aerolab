#!/bin/bash
set -e

# Remove old aerolab-managed entries
sed -i.bak '/# aerolab-managed/d' /etc/hosts

# Add new entries
cat >> /etc/hosts << 'EOF'
{{HOSTS_ENTRIES}}
EOF

