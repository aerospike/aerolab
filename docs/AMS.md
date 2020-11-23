# How to deploy aerospike monitoring stack

## Deply aerospike node with tls, a clean ubuntu container and generate/copy TLS certs

```
% aerolab make-cluster -v 4.9.0.10 -o templates/tls.conf
% aerolab gen-tls-certs
% aerolab stop-aerospike ; sleep 10 ; aerolab start-aerospike
% aerolab deploy-container
% aerolab copy-tls-certs -s mydc -d container -t tls1
% aerolab cluster-list |grep mydc |egrep -o '172.*'
% NODEIP=$(aerolab cluster-list |grep mydc |egrep -o '172.*' |head -1)
% aerolab node-attach -n container -- bash -c "export NODEIP=$NODEIP && bash"
```

## Install basic required components and exporter

```
$ apt-get update && apt-get -y install systemd screen git wget
$ wget https://www.aerospike.com/download/monitoring/aerospike-prometheus-exporter/latest/artifact/deb -O aerospike-prometheus-exporter.tgz
$ tar -xvzf aerospike-prometheus-exporter.tgz
$ dpkg -i aerospike-prometheus-exporter-*-amd64.deb
```

## Configure exporter

```
$ sed -i 's/db_port=3000/db_port=4333/g' /etc/aerospike-prometheus-exporter/ape.toml
$ sed -i 's/cert_file=""/cert_file="\/etc\/aerospike\/ssl\/tls1\/cert.pem"/g' /etc/aerospike-prometheus-exporter/ape.toml
$ sed -i 's/key_file=""/key_file="\/etc\/aerospike\/ssl\/tls1\/key.pem"/g' /etc/aerospike-prometheus-exporter/ape.toml
$ sed -i 's/root_ca=""/root_ca="\/etc\/aerospike\/ssl\/tls1\/cacert.pem"/g' /etc/aerospike-prometheus-exporter/ape.toml
$ sed -i 's/node_tls_name=""/node_tls_name="tls1"/g' /etc/aerospike-prometheus-exporter/ape.toml
$ sed -i "s/db_host=\"localhost\"/db_host=\"$NODEIP\"/g" /etc/aerospike-prometheus-exporter/ape.toml
```

## Start exporter

```
$ screen -dmS exporter /usr/bin/aerospike-prometheus-exporter --config /etc/aerospike-prometheus-exporter/ape.toml
```

## To attach to exporter to see it's console

```
$ screen -r exporter
```

## To detach from exporter, press ctrl+a,d (first control+a and then let go of control button and press d)

## Install prometheus and perform basic configuration

```
$ apt-get -y install prometheus prometheus-alertmanager
$ sed -i 's/rule_files:/rule_files:\n  - "\/etc\/prometheus\/aerospike_rules.yaml"/g' /etc/prometheus/prometheus.yml
$ cat <<EOF >> /etc/prometheus/prometheus.yml
  - job_name: 'aerospike'
    static_configs:
      - targets: ['127.0.0.1:9145']
EOF
```

## Configure aerospike_rules.yaml and start prometheus

```
$ git clone https://github.com/aerospike/aerospike-monitoring.git
$ cp aerospike-monitoring/config/prometheus/aerospike_rules.yml /etc/prometheus/aerospike_rules.yaml
$ service prometheus start
```

## Install grafana

```
$ apt-get install -y apt-transport-https
$ apt-get install -y software-properties-common wget
$ wget -q -O - https://packages.grafana.com/gpg.key | apt-key add -
$ echo "deb https://packages.grafana.com/oss/deb stable main" | tee -a /etc/apt/sources.list.d/grafana.list
$ apt-get update && apt-get -y install grafana
```

## Configure grafana dashboards and datasources

```
$ sed -i 's/;provisioning .*/provisioning = \/etc\/grafana\/provisioning/g' /etc/grafana/grafana.ini
$ mkdir -p /etc/grafana/provisioning/datasources
$ mkdir -p /etc/grafana/provisioning/dashboards
$ cp aerospike-monitoring/config/grafana/provisioning/datasources/all.yaml /etc/grafana/provisioning/datasources/
$ cp aerospike-monitoring/config/grafana/provisioning/dashboards/all.yaml /etc/grafana/provisioning/dashboards/
$ grafana-cli plugins install camptocamp-prometheus-alertmanager-datasource
$ mkdir /var/lib/grafana/dashboards
$ cp aerospike-monitoring/config/grafana/dashboards/* /var/lib/grafana/dashboards/
$ sed -i 's/prometheus:9090/localhost:9090/g' /etc/grafana/provisioning/datasources/all.yaml
```

## (re)Start grafana

```
$ service grafana-server stop; service grafana-server start
```

## Get IP of grafana/prometheus stack

```
$ ip addr sh eth0 |egrep -o '172\.[0-9]+\.[0-9]+\.[0-9]+' |head -1
```

## Now visit http://[IP]:3000

* user/pass: admin/admin

## Remove everything:

```
% aerolab cluster-destroy -f -n container; aerolab cluster-destroy -f
```
