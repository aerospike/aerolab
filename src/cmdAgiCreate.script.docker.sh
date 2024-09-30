set -e
override=%s
mkdir -p /opt/agi/aerospike/data
mkdir -p /opt/agi/aerospike/smd
[ "%t" = "true" ] && touch /opt/agi/nodim || echo "DIM"
cat <<'EOF' > /opt/agi/owner
%s
EOF
set +e
which apt
ISAPT=$?
set -e
if [ $ISAPT -eq 0 ]
then
    apt update && apt -y install wget adduser libfontconfig1 musl ssl-cert && wget -q https://dl.grafana.com/oss/release/grafana_%s_%s.deb && dpkg -i grafana_%s_%s.deb
else
    yum install -y wget mod_ssl
    mkdir -p /etc/ssl/certs /etc/ssl/private
    openssl req -new -x509 -nodes -out /etc/ssl/certs/ssl-cert-snakeoil.pem -keyout /etc/ssl/private/ssl-cert-snakeoil.key -days 3650 -subj '/CN=www.example.com'
    yum install -y https://dl.grafana.com/oss/release/grafana-%s-1.%s.rpm
fi
[ ! -f /opt/agi/proxy.cert ] && cp /etc/ssl/certs/ssl-cert-snakeoil.pem /opt/agi/proxy.cert
[ ! -f /opt/agi/proxy.key ] && cp /etc/ssl/private/ssl-cert-snakeoil.key /opt/agi/proxy.key
chmod 755 /usr/local/bin/aerolab
mkdir /opt/autoload
aerolab config backend -t none
%s
cat <<'EOF' > /etc/aerospike/aerospike.conf
service {
    proto-fd-max 15000
    work-directory /opt/agi/aerospike
    cluster-name agi
}
logging {
    file /var/log/agi-aerospike.log {
        context any info
    }
}
network {
    service {
        address any
        port 3000
    }
    heartbeat {
        interval 150
        mode mesh
        port 3002
        timeout 10
    }
    fabric {
        port 3001
    }
    info {
        port 3003
    }
}
namespace agi {
    default-ttl 0
    %s
    replication-factor 2
    storage-engine %s {
        file /opt/agi/aerospike/data/agi.dat
        filesize %dG
        %s
        %s
        %s
    }
}
EOF

if [ $override -eq 1 -o ! -f /opt/autoload/plugin.sh ]
then
cat <<'EOF' > /opt/autoload/plugin.sh
nohup /usr/local/bin/aerolab agi exec plugin -y /opt/agi/plugin.yaml >>/var/log/agi-plugin.log 2>&1 &
EOF
fi

if [ $override -eq 1 -o ! -f /opt/autoload/grafanafix.sh ]
then
cat <<'EOF' > /opt/autoload/grafanafix.sh
nohup /usr/local/bin/aerolab agi exec grafanafix -y /opt/agi/grafanafix.yaml >>/var/log/agi-grafanafix.log 2>&1 &
EOF
fi

if [ $override -eq 1 -o ! -f /opt/autoload/ingest.sh ]
then
cat <<'EOF' > /opt/autoload/ingest.sh
nohup /usr/local/bin/aerolab agi exec ingest -y /opt/agi/ingest.yaml --agi-name %s >>/var/log/agi-ingest.log 2>&1 &
EOF
fi

if [ $override -eq 1 -o ! -f /opt/autoload/proxy.sh ]
then
cat <<'EOF' > /opt/autoload/proxy.sh
nohup /usr/local/bin/aerolab agi exec proxy -c "/usr/bin/touch /tmp/poweroff.now" --agi-name %s -L "%s" -a token -l %d %s -C %s -K %s -m %s -M %s >>/var/log/agi-proxy.log 2>&1 &
EOF
fi

if [ $override -eq 1 -o ! -f /opt/agi/grafanafix.yaml ]
then
cat <<'EOF' > /opt/agi/grafanafix.yaml
dashboards:
  fromDir: ""
  loadEmbedded: true
grafanaURL: "http://127.0.0.1:8850"
annotationFile: "/opt/agi/annotations.json"
EOF
fi

if [ $override -eq 1 -o ! -f /opt/agi/plugin.yaml ]
then
cat <<'EOF' > /opt/agi/plugin.yaml
maxDataPointsReceived: %d
logLevel: %d
addNoneToLabels:
  - Histogram
  - HistogramDev
  - HistogramUs
  - HistogramSize
  - HistogramCount
%s
EOF
fi

if [ $override -eq 1 -o ! -f /opt/agi/notifier.yaml ]
then
cat <<'EOF' > /opt/agi/notifier.yaml
%s
EOF
fi

if [ $override -eq 1 -o ! -f /opt/agi/ingest.yaml ]
then
    mv /tmp/ingest.yaml /opt/agi/ingest.yaml
fi

if [ $override -eq 1 -o ! -f /opt/agi/deployment.json.gz ]
then
    mv /tmp/deployment.json.gz /opt/agi/deployment.json.gz
fi
rm -f /tmp/ingest.yaml /tmp/deployment.json.gz

chmod 755 /opt/autoload/*
rm -rf /root/agiinstaller.sh && exit 0 || exit 0
