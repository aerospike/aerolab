. configure.sh

cat <<'BOF' > getloki.sh
apt-get update && apt-get -y install wget curl unzip || exit 1
cd /root
wget https://github.com/grafana/loki/releases/download/v2.5.0/loki-linux-amd64.zip || exit 1
unzip loki-linux-amd64.zip || exit 1
mv loki-linux-amd64 /usr/bin/loki || exit 1
wget https://github.com/grafana/loki/releases/download/v2.5.0/logcli-linux-amd64.zip || exit 1
unzip logcli-linux-amd64.zip || exit 1
mv logcli-linux-amd64 /usr/bin/logcli || exit 1
chmod 755 /usr/bin/logcli /usr/bin/loki || exit 1

mkdir -p /etc/loki /data-logs/loki
cat <<'EOF' > /etc/loki/loki.yaml
auth_enabled: false

server:
  http_listen_port: 3100
  grpc_listen_port: 9096

common:
  path_prefix: /data-logs/loki
  storage:
    filesystem:
      chunks_directory: /data-logs/loki/chunks
      rules_directory: /data-logs/loki/rules
  replication_factor: 1
  ring:
    instance_addr: 127.0.0.1
    kvstore:
      store: inmemory

schema_config:
  configs:
    - from: 2020-10-24
      store: boltdb-shipper
      object_store: filesystem
      schema: v11
      index:
        prefix: index_
        period: 24h

analytics:
  reporting_enabled: false
EOF

cat <<'EOF' > /opt/start-loki.sh
nohup /usr/bin/loki -config.file=/etc/loki/loki.yaml -log-config-reverse-order > /var/log/loki.log 2>&1 &
EOF
BOF

cat <<'BOF' > getgrafana.sh
apt-get install -y apt-transport-https || exit 1
apt-get install -y software-properties-common wget || exit 1
wget -q -O /usr/share/keyrings/grafana.key https://apt.grafana.com/gpg.key || exit 1
echo "deb [signed-by=/usr/share/keyrings/grafana.key] https://apt.grafana.com stable main" > /etc/apt/sources.list.d/grafana.list
apt-get update || exit 1
apt-get -y install grafana || exit 1
cat <<'EOF' > /opt/start-grafana.sh
systemctl daemon-reload || echo "not-systemd"
sleep 3
service grafana-server stop || echo "already stopped"
sleep 5
service grafana-server start || echo "Started successfully"
EOF
BOF

cat <<'BOF' > getpromtail.sh
[ ! -f /var/log/asbench.log ] && echo "tadaa" > /var/log/asbench.log
apt-get update
apt-get -y install unzip 
cd /root
wget https://github.com/grafana/loki/releases/download/v2.5.0/promtail-linux-amd64.zip
unzip promtail-linux-amd64.zip
mv promtail-linux-amd64 /usr/bin/promtail
chmod 755 /usr/bin/promtail
mkdir -p /etc/promtail /var/promtail
cat <<EOF > /etc/promtail/promtail.yaml
server:
  http_listen_port: 9080
  grpc_listen_port: 0

positions:
  filename: /var/promtail/positions.yaml

clients:
  - url: http://$(cat /opt/asbench-grafana.ip):3100/loki/api/v1/push

scrape_configs:
  - job_name: asbench
    static_configs:
      - targets:
          - localhost
        labels:
          job: asbench
          __path__: /var/log/asbench.log
          host: $(hostname)
EOF
cat <<'EOF' > /opt/start-promtail.sh
nohup /usr/bin/promtail -config.file=/etc/promtail/promtail.yaml -log-config-reverse-order > /var/log/promtail.log 2>&1 &
EOF
BOF

aerolab client create base -n ${PRETTY_NAME} -c 1 --instance-type ${AWS_CLIENT_INSTANCE} --ebs=40 --secgroup-id=${us_west_2_open} --subnet-id=${us_west_2a} || exit 1

aerolab client list |grep ${PRETTY_NAME} |egrep -o '([0-9]{1,3}\.){3}[0-9]{1,3}' |head -1 > asbench-grafana.ip
cat asbench-grafana.ip |egrep -o '([0-9]{1,3}\.){3}[0-9]{1,3}' || exit 2
aerolab files upload -c -n ${CLIENT_NAME} asbench-grafana.ip /opt/asbench-grafana.ip || exit 1
rm -f asbench-grafana.ip

aerolab files upload -c -n ${PRETTY_NAME} getloki.sh /root/getloki.sh || exit 1
aerolab files upload -c -n ${PRETTY_NAME} getgrafana.sh /root/getgrafana.sh || exit 1
aerolab files upload -c -n ${CLIENT_NAME} getpromtail.sh /root/getpromtail.sh || exit 1
rm -f getloki.sh getgrafana.sh getpromtail.sh

aerolab client attach -n ${PRETTY_NAME} -- bash /root/getloki.sh || exit 1
aerolab client attach -n ${PRETTY_NAME} -- bash /root/getgrafana.sh || exit 1
aerolab client attach -n ${PRETTY_NAME} --detach -- bash /opt/start-loki.sh || exit 1
aerolab client attach -n ${PRETTY_NAME} --detach -- bash /opt/start-grafana.sh || exit 1

aerolab client attach -n ${CLIENT_NAME} -l all -- bash /root/getpromtail.sh || exit 1
aerolab client attach -n ${CLIENT_NAME} -l all --detach -- bash /opt/start-promtail.sh || exit 1

echo
nip=$(aerolab client list |grep ${PRETTY_NAME} |egrep -o '([0-9]{1,3}\.){3}[0-9]{1,3}' |tail -1)
echo "1. Go to http://${nip}:3000 and login (admin:admin)"
echo "2. Go to http://${nip}:3000/datasources , click on 'Add data source' and add Loki with URL http://localhost:3100 (enter URL and hit 'Save and Test')"
echo "3. Go to http://${nip}:3000/dashboard/import , click on 'Upload JSON file' and upload asbench.json provided in this directory"
echo "4. Go to http://${nip}:3000/d/Ck3pRJnVz/asbench-statistics and enjoy"

