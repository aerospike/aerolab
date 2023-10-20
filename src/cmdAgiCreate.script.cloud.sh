set -e
mkdir -p /opt/agi/aerospike/data
apt update && apt -y install wget adduser libfontconfig1 musl ssl-cert && wget -q https://dl.grafana.com/oss/release/grafana_10.1.2_%s.deb && dpkg -i grafana_10.1.2_%s.deb
chmod 755 /usr/local/bin/aerolab
aerolab config backend -t none
cat <<'EOF' > /etc/aerospike/aerospike.conf
service {
    proto-fd-max 15000
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
cat <<'EOF' > /etc/systemd/system/agi-plugin.service
[Unit]
Description=AGI Plugin
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=on-failure
User=root
RestartSec=10
WorkingDirectory=/opt/agi
ExecStart=/usr/local/bin/aerolab agi exec plugin -y /opt/agi/plugin.yaml

[Install]
WantedBy=multi-user.target
EOF
cat <<'EOF' > /etc/systemd/system/agi-grafanafix.service
[Unit]
Description=AGI Grafana Helper
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=on-failure
User=root
RestartSec=10
WorkingDirectory=/opt/agi
ExecStart=/usr/local/bin/aerolab agi exec grafanafix -y /opt/agi/grafanafix.yaml

[Install]
WantedBy=multi-user.target
EOF
cat <<'EOF' > /etc/systemd/system/agi-ingest.service
[Unit]
Description=AGI Ingest
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=never
User=root
RestartSec=10
WorkingDirectory=/opt/agi
ExecStart=/usr/local/bin/aerolab agi exec ingest -y /opt/agi/ingest.yaml

[Install]
WantedBy=multi-user.target
EOF
cat <<'EOF' > /etc/systemd/system/agi-proxy.service
[Unit]
Description=AGI Proxy
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=on-failure
User=root
RestartSec=10
WorkingDirectory=/opt/agi
ExecStart=/usr/local/bin/aerolab agi exec proxy -L "%s" -a token -l %d %s -C %s -K %s -m %s -M %s

[Install]
WantedBy=multi-user.target
EOF
cat <<'EOF' > /opt/agi/grafanafix.yaml
dashboards:
  fromDir: ""
  loadEmbedded: true
grafanaURL: "http://127.0.0.1:8850"
annotationFile: "/opt/agi/annotations.json"
EOF
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
cat <<'EOF' > /opt/agi/notifier.yaml
%s
EOF
chmod 755 /etc/systemd/system/agi-plugin.service /etc/systemd/system/agi-proxy.service /etc/systemd/system/agi-grafanafix.service /etc/systemd/system/agi-ingest.service
systemctl enable agi-plugin && systemctl enable agi-proxy && systemctl enable agi-grafanafix && systemctl enable agi-ingest && systemctl daemon-reload
systemctl start agi-plugin ; systemctl start agi-proxy ; systemctl start agi-grafanafix ; systemctl start agi-ingest
rm -rf /root/agiinstaller.sh && exit 0 || exit 0
