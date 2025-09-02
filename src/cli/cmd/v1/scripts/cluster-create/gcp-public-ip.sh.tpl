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
INTIP=""; EXTIP=""
attempts=0
max=120
while [ "${INTIP}" = "" ]
do
	INTIP=$(curl -H "Metadata-Flavor: Google" http://169.254.169.254/computeMetadata/v1/instance/network-interfaces/0/ip)
	[ "${INTIP}" = "" ] && sleep 1 || break
	attempts=$(( $attempts + 1 ))
	[ $attempts -eq $max ] && exit 1
done
while [ "${EXTIP}" = "" ]
do
	EXTIP=$(curl -H "Metadata-Flavor: Google" http://169.254.169.254/computeMetadata/v1/instance/network-interfaces/0/access-configs/0/external-ip)
	[ "${EXTIP}" = "" ] && sleep 1 || break
	attempts=$(( $attempts + 1 ))
	[ $attempts -eq $max ] && exit 1
done
grep 'alternate-access-address' /etc/aerospike/aerospike.conf
if [ $? -ne 0 ]
then
	sed -i 's/address any/address any\naccess-address\nalternate-access-address\ntls-access-address\ntls-alternate-access-address\n/g' /etc/aerospike/aerospike.conf
fi
sed -e "s/access-address.*/access-address ${INTIP}/g" -e "s/alternate-access-address.*/alternate-access-address ${EXTIP}/g"  /etc/aerospike/aerospike.conf > ~/aerospike.conf.new && cp /etc/aerospike/aerospike.conf /etc/aerospike/aerospike.conf.bck && cp ~/aerospike.conf.new /etc/aerospike/aerospike.conf
EOF

chmod 755 /usr/local/bin/aerospike-access-address.sh
chmod 755 /etc/systemd/system/aerospike-access-address.service
systemctl daemon-reload
systemctl enable aerospike-access-address
systemctl start aerospike-access-address
