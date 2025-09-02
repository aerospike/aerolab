mkdir -p /etc/systemd/resolved.conf.d
cat <<'EOF' > /etc/systemd/resolved.conf.d/aerolab.conf
[Resolve]
DNS=1.1.1.1
FallbackDNS=8.8.8.8
EOF
systemctl restart systemd-resolved || echo "No systemctl"
if [ -d /etc/NetworkManager/system-connections ]
then
ls /etc/NetworkManager/system-connections |sed 's/.nmconnection//g' |while read file; do nmcli conn modify "$file" ipv4.dns "1.1.1.1 8.8.8.8"; done
systemctl restart NetworkManager
fi
cat <<'EOF' > /etc/resolv.conf
nameserver 1.1.1.1
nameserver 8.8.8.8
EOF
