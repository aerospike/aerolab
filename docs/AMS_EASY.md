# Deploying 1 aerospike and 1 AMS node

## On your laptop

```
aerolab make-cluster -v 5.6.0.3
aerolab deploy-container
aerolab cluster-list |grep mydc |egrep -o '172.*'
NODEIP=$(aerolab cluster-list |grep mydc |egrep -o '172.*' |head -1)
aerolab node-attach -n container -- bash -c "export NODEIP=$NODEIP && bash"
```

## Now inside the AMS node

```
apt-get update && apt-get -y install systemd screen git wget
wget https://www.aerospike.com/download/monitoring/aerospike-prometheus-exporter/latest/artifact/deb -O aerospike-prometheus-exporter.tgz
tar -xvzf aerospike-prometheus-exporter.tgz
dpkg -i aerospike-prometheus-exporter-*-amd64.deb
sed -i "s/db_host=\"localhost\"/db_host=\"$NODEIP\"/g" /etc/aerospike-prometheus-exporter/ape.toml
screen -dmS exporter /usr/bin/aerospike-prometheus-exporter --config /etc/aerospike-prometheus-exporter/ape.toml
apt-get -y install prometheus prometheus-alertmanager
sed -i 's/rule_files:/rule_files:\n  - "\/etc\/prometheus\/aerospike_rules.yaml"/g' /etc/prometheus/prometheus.yml
cat <<EOF >> /etc/prometheus/prometheus.yml
  - job_name: 'aerospike'
    static_configs:
      - targets: ['127.0.0.1:9145']
EOF
git clone https://github.com/aerospike/aerospike-monitoring.git
cp aerospike-monitoring/config/prometheus/aerospike_rules.yml /etc/prometheus/aerospike_rules.yaml
service prometheus start
apt-get install -y apt-transport-https software-properties-common wget
wget -q -O - https://packages.grafana.com/gpg.key | apt-key add -
echo "deb https://packages.grafana.com/oss/deb stable main" | tee -a /etc/apt/sources.list.d/grafana.list
apt-get update && apt-get -y install grafana
sed -i 's/;provisioning .*/provisioning = \/etc\/grafana\/provisioning/g' /etc/grafana/grafana.ini
mkdir -p /etc/grafana/provisioning/datasources
mkdir -p /etc/grafana/provisioning/dashboards
cp aerospike-monitoring/config/grafana/provisioning/datasources/all.yaml /etc/grafana/provisioning/datasources/
cp aerospike-monitoring/config/grafana/provisioning/dashboards/all.yaml /etc/grafana/provisioning/dashboards/
grafana-cli plugins install camptocamp-prometheus-alertmanager-datasource
mkdir /var/lib/grafana/dashboards
cp aerospike-monitoring/config/grafana/dashboards/* /var/lib/grafana/dashboards/
sed -i 's/prometheus:9090/localhost:9090/g' /etc/grafana/provisioning/datasources/all.yaml
service grafana-server stop; service grafana-server start
ip addr sh eth0 |egrep -o '172\.[0-9]+\.[0-9]+\.[0-9]+' |head -1
```

## Connect

You can now connect to graphana on port 3000

## Insert data

```
aerolab insert-data -z 100000
```

## Destroy everything

```
aerolab cluster-destroy -f -n container; aerolab cluster-destroy -f
```
