cat <<'EOF' > /etc/systemd/system/aerospike-early.service
[Unit]
Description=Run early script before Aerospike starts
RequiredBy=aerospike.service
Before=aerospike.service
		
[Service]
Type=oneshot
ExecStart=/bin/bash /usr/local/bin/early.sh
		
[Install]
WantedBy=multi-user.target
EOF

cat <<'EOF' > /usr/local/bin/early.sh
#!/bin/bash
exit 0
EOF

cat <<'EOF' > /etc/systemd/system/aerospike-late.service
[Unit]
Description=Run late script before Aerospike starts
RequiredBy=aerospike.service
Before=aerospike.service
		
[Service]
Type=oneshot
ExecStart=/bin/bash /usr/local/bin/late.sh
		
[Install]
WantedBy=multi-user.target
EOF

cat <<'EOF' > /usr/local/bin/late.sh
#!/bin/bash
exit 0
EOF

chmod 755 /usr/local/bin/early.sh
chmod 755 /usr/local/bin/late.sh
chmod 755 /etc/systemd/system/aerospike-early.service
chmod 755 /etc/systemd/system/aerospike-late.service
systemctl daemon-reload
