cat <<'EOF' > /etc/systemd/system/aerospike.service.d/aerolab-thp.conf
[Service]
ExecStartPre=/bin/bash -c "echo 'never' > /sys/kernel/mm/transparent_hugepage/enabled || echo"
ExecStartPre=/bin/bash -c "echo 'never' > /sys/kernel/mm/transparent_hugepage/defrag || echo"
ExecStartPre=/bin/bash -c "echo 'never' > /sys/kernel/mm/redhat_transparent_hugepage/enabled || echo"
ExecStartPre=/bin/bash -c "echo 'never' > /sys/kernel/mm/redhat_transparent_hugepage/defrag || echo"
ExecStartPre=/bin/bash -c "echo 0 > /sys/kernel/mm/transparent_hugepage/khugepaged/defrag || echo"
ExecStartPre=/bin/bash -c "echo 0 > /sys/kernel/mm/redhat_transparent_hugepage/khugepaged/defrag || echo"
ExecStartPre=/bin/bash -c "sysctl -w vm.min_free_kbytes=1310720 || echo"
ExecStartPre=/bin/bash -c "sysctl -w vm.swappiness=0 || echo"
EOF
chmod 755 /etc/systemd/system/aerospike.service.d/aerolab-thp.conf
