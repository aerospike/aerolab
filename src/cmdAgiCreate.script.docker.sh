set -e
override=%s
mkdir -p /opt/agi/aerospike/data
mkdir -p /opt/agi/aerospike/smd
cat <<'EOF' > /opt/agi/owner
%s
EOF
apt update && apt -y install wget adduser libfontconfig1 musl ssl-cert && wget -q https://dl.grafana.com/oss/release/grafana_10.1.2_%s.deb && dpkg -i grafana_10.1.2_%s.deb
chmod 755 /usr/local/bin/aerolab
mkdir /opt/autoload
aerolab config backend -t none
cat <<'EOF' > /etc/aerospike/aerospike.conf
service {
    proto-fd-max 15000
    work-directory /opt/agi/aerospike
}
logging {
    console {
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
    memory-size %dG
    replication-factor 2
    storage-engine device {
        file /opt/agi/aerospike/data/agi.dat
        filesize %dG
        data-in-memory true
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
chmod 755 /opt/autoload/*
rm -rf /root/agiinstaller.sh && exit 0 || exit 0
