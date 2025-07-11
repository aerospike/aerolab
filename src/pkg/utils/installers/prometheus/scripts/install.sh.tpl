# shellcheck disable=SC2148

VERSION="{{.Version}}"
DOWNLOAD_URL_ARM64="{{.DownloadURLARM64}}"
DOWNLOAD_URL_AMD64="{{.DownloadURLAMD64}}"
ENABLE_PROMETHEUS="{{.EnablePrometheus}}"
START_PROMETHEUS="{{.StartPrometheus}}"

DOWNLOAD_URL=${DOWNLOAD_URL_AMD64}
ARCH=amd64
if [ "$(uname -m)" == "aarch64" ] || [ "$(uname -m)" == "arm64" ]; then
    DOWNLOAD_URL=${DOWNLOAD_URL_ARM64}
    ARCH=arm64
fi

set -e
pushd /tmp
curl -L -o prometheus-${VERSION}.linux-${ARCH}.tar.gz ${DOWNLOAD_URL}
tar -zxvf prometheus-${VERSION}.linux-${ARCH}.tar.gz
cd prometheus-${VERSION}.linux-${ARCH}/
mv promtool /usr/local/bin/
mv prometheus /usr/local/bin/
mkdir -p /etc/prometheus
mv prometheus.yml /etc/prometheus/
mkdir -p /var/lib/prometheus/data
mkdir -p /usr/local/share/prometheus/consoles
mkdir -p /usr/local/share/prometheus/console_libraries
cat <<'EOF' > /usr/lib/systemd/system/prometheus.service
[Unit]
Description=Prometheus
After=network.target
[Service]
Type=simple
ExecStart=/usr/local/bin/prometheus --config.file=/etc/prometheus/prometheus.yml --storage.tsdb.path=/var/lib/prometheus/data --web.console.templates=/usr/local/share/prometheus/consoles --web.console.libraries=/usr/local/share/prometheus/console_libraries
Restart=always
WorkingDirectory=/var/lib/prometheus

[Install]
WantedBy=multi-user.target
EOF

set +e
systemctl daemon-reload || true
if [ "$ENABLE_PROMETHEUS" == "true" ]; then
    systemctl enable prometheus || exit 1
fi
if [ "$START_PROMETHEUS" == "true" ]; then
    systemctl start prometheus || exit 1
fi
popd
