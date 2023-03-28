# Example script to install kibana

The script install aerospike cluster, elasticsearch cluster, elasticsearch connection to aerospike, and kibana connected to elasticsearch.

```bash
set -e

# naming
ES="myes"
CLUSTER="bob"
KIBANA="kibana"

# aerospike, elasticsearch
aerolab cluster create -c 2 -v 6.2.0.6 -n ${CLUSTER}
aerolab client create elasticsearch -c 2 -r 4 -n ${ES}
aerolab xdr connect -S ${CLUSTER} -D ${ES} -c

# kibana
aerolab client create base -n ${KIBANA}
cat <<'EOF' > kibana-install.sh
set -e
apt-get update
apt-get -y install apt-transport-https wget gpg
wget -qO - https://artifacts.elastic.co/GPG-KEY-elasticsearch | gpg --dearmor -o /usr/share/keyrings/elasticsearch-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/elasticsearch-keyring.gpg] https://artifacts.elastic.co/packages/8.x/apt stable main" > /etc/apt/sources.list.d/elastic-8.x.list
apt-get update && apt-get -y install kibana
sed -i 's/#server.port: 5601/server.port: 5601/g' /etc/kibana/kibana.yml
sed -i 's/#server.host: "localhost"/server.host: "0.0.0.0"/g' /etc/kibana/kibana.yml
mkdir -p /opt/autoload
echo 'nohup /usr/share/kibana/bin/kibana --allow-root >> /var/log/kibana-out.log 2>&1 &' > /opt/autoload/01-kibana
chmod 755 /opt/autoload/01-kibana
EOF
aerolab files upload -n ${KIBANA} -c kibana-install.sh /root/install.sh
rm -f kibana-install.sh
aerolab attach client -n ${KIBANA} -- /bin/bash /root/install.sh
token=$(aerolab attach client -n ${ES} -- /usr/share/elasticsearch/bin/elasticsearch-create-enrollment-token -s kibana)
aerolab attach client -n ${KIBANA} -- /usr/share/kibana/bin/kibana-setup -t ${token}
aerolab attach client -n ${KIBANA} --detach -- /bin/bash /opt/autoload/01-kibana
KIBANA_IP=$(aerolab client list -i |grep client=kibana |egrep -o "ext_ip=.*" |awk -F'=' '{print $2}')
echo "Access kibana via: http://${KIBANA_IP}:5601"
echo "It may take a minute for kibana to come up, be patient!"
echo "Username/password is: elastic/elastic"
```
