cat <<'EOF' > /etc/systemd/system/aerospike-access-address.service
[Unit]
Description=Fix Aerospike access-address and alternate-access-address
RequiredBy=aerospike.service
Before=aerospike.service
		
[Service]
Type=oneshot
ExecStart=/bin/bash /usr/local/bin/aerospike-access-address.sh
		
[Install]
WantedBy=multi-user.target
EOF

cat <<'EOF' > /usr/local/bin/aerospike-access-address.sh
grep 'alternate-access-address' /etc/aerospike/aerospike.conf
if [ $? -ne 0 ]
then
	sed -i 's/address any/address any\naccess-address\nalternate-access-address\ntls-access-address\ntls-alternate-access-address\n/g' /etc/aerospike/aerospike.conf
fi
T=$(curl -sS -X PUT http://169.254.169.254/latest/api/token -H "X-aws-ec2-metadata-token-ttl-seconds: 21600")
PUBIP=$(curl -sS -H "X-aws-ec2-metadata-token: $T" http://169.254.169.254/latest/meta-data/public-ipv4)
PRIVIP=$(curl -sS -H "X-aws-ec2-metadata-token: $T" http://169.254.169.254/latest/meta-data/local-ipv4)
sed -e "s/access-address.*/access-address $PRIVIP/g" -e "s/alternate-access-address.*/alternate-access-address $PUBIP/g"  /etc/aerospike/aerospike.conf > ~/aerospike.conf.new && cp /etc/aerospike/aerospike.conf /etc/aerospike/aerospike.conf.bck && cp ~/aerospike.conf.new /etc/aerospike/aerospike.conf
EOF

chmod 755 /usr/local/bin/aerospike-access-address.sh
chmod 755 /etc/systemd/system/aerospike-access-address.service
systemctl daemon-reload
systemctl enable aerospike-access-address
systemctl start aerospike-access-address
