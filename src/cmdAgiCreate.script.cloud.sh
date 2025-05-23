set -e
override=%s
mkdir -p /opt/agi/aerospike/data
mkdir -p /opt/agi/aerospike/smd
uuidgen -r > /opt/agi/uuid
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
    ANS=1
    TOUT=0
    while [ $ANS -ne 0 ]
    do
        apt update && apt -y install wget adduser libfontconfig1 musl ssl-cert && wget -q https://dl.grafana.com/oss/release/grafana_%s_%s.deb && dpkg -i grafana_%s_%s.deb
        ANS=$?
        if [ $ANS -ne 0 ]
        then
            TOUT=$((TOUT + 1))
            [ $TOUT -gt 900 ] && exit 1
            sleep 1
        fi
    done
else
    yum install -y wget mod_ssl
    mkdir -p /etc/ssl/certs /etc/ssl/private
    openssl req -new -x509 -nodes -out /etc/ssl/certs/ssl-cert-snakeoil.pem -keyout /etc/ssl/private/ssl-cert-snakeoil.key -days 3650 -subj '/CN=www.example.com'
    yum install -y https://dl.grafana.com/oss/release/grafana-%s-1.%s.rpm
fi
[ ! -f /opt/agi/proxy.cert ] && cp /etc/ssl/certs/ssl-cert-snakeoil.pem /opt/agi/proxy.cert
[ ! -f /opt/agi/proxy.key ] && cp /etc/ssl/private/ssl-cert-snakeoil.key /opt/agi/proxy.key
chmod 755 /usr/local/bin/aerolab
aerolab config backend -t none
%s
[ ! -f /opt/agi/aerospike/features.conf ] && cp /etc/aerospike/features.conf /opt/agi/aerospike/features.conf
cat <<'EOF' > /etc/aerospike/aerospike.conf
service {
    proto-fd-max 15000
    work-directory /opt/agi/aerospike
    cluster-name agi
    feature-key-file /opt/agi/aerospike/features.conf
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
        %s
    }
}
EOF

echo "%s" > /opt/agi/label
echo "%s" > /opt/agi/name

if [ $override -eq 1 -o ! -f /etc/systemd/system/agi-plugin.service ]
then
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
fi

if [ $override -eq 1 -o ! -f /etc/systemd/system/agi-grafanafix.service ]
then
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
fi

if [ $override -eq 1 -o ! -f /etc/systemd/system/agi-ingest.service ]
then
cat <<'EOF' > /etc/systemd/system/agi-ingest.service
[Unit]
Description=AGI Ingest
After=network.target

[Service]
Type=simple
TimeoutStopSec=600
Restart=no
User=root
RestartSec=10
WorkingDirectory=/opt/agi
ExecStart=/usr/local/bin/aerolab agi exec ingest -y /opt/agi/ingest.yaml --agi-name %s

[Install]
WantedBy=multi-user.target
EOF
fi

if [ $override -eq 1 -o ! -f /etc/systemd/system/agi-proxy.service ]
then
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
ExecStart=/usr/local/bin/aerolab agi exec proxy -c "%s" --agi-name %s -L "%s" -a token -l %d %s -C %s -K %s -m %s -M %s

[Install]
WantedBy=multi-user.target
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
labelFiles:
  - "/opt/agi/label"
  - "/opt/agi/name"
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

chmod 755 /etc/systemd/system/agi-plugin.service /etc/systemd/system/agi-proxy.service /etc/systemd/system/agi-grafanafix.service /etc/systemd/system/agi-ingest.service
systemctl enable agi-plugin && systemctl enable agi-proxy && systemctl enable agi-grafanafix && systemctl enable agi-ingest && systemctl daemon-reload
systemctl start agi-plugin ; systemctl start agi-proxy ; systemctl start agi-grafanafix ; systemctl start agi-ingest
set +e
rm -rf /root/agiinstaller.sh

cat <<'EOF'> /usr/local/bin/erro
grep -i 'error|warn|timeout' "$@"
EOF
chmod 755 /usr/local/bin/erro
date > /opt/agi-installed && exit 0 || exit 0
