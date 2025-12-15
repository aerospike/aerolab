# shellcheck disable=SC2148

VERSION="{{.Version}}"
DOWNLOAD_URL_ARM64="{{.DownloadURLARM64}}"
DOWNLOAD_URL_AMD64="{{.DownloadURLAMD64}}"
ENABLE_NODE_EXPORTER="{{.EnableNodeExporter}}"
START_NODE_EXPORTER="{{.StartNodeExporter}}"

DOWNLOAD_URL=${DOWNLOAD_URL_AMD64}
ARCH=amd64
if [ "$(uname -m)" == "aarch64" ] || [ "$(uname -m)" == "arm64" ]; then
    DOWNLOAD_URL=${DOWNLOAD_URL_ARM64}
    ARCH=arm64
fi

set -e
pushd /tmp
curl -L -o node_exporter-${VERSION}.linux-${ARCH}.tar.gz ${DOWNLOAD_URL}
tar -zxvf node_exporter-${VERSION}.linux-${ARCH}.tar.gz
cd node_exporter-${VERSION}.linux-${ARCH}/
mv node_exporter /usr/local/bin/
chmod +x /usr/local/bin/node_exporter

# Create systemd service (runs as root for compatibility)
mkdir -p /usr/lib/systemd/system
cat <<'EOF' > /usr/lib/systemd/system/node_exporter.service
[Unit]
Description=Node Exporter
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/node_exporter
Restart=always

[Install]
WantedBy=multi-user.target
EOF

set +e
systemctl daemon-reload || true
if [ "$ENABLE_NODE_EXPORTER" == "true" ]; then
    systemctl enable node_exporter || exit 1
fi
if [ "$START_NODE_EXPORTER" == "true" ]; then
    systemctl start node_exporter || exit 1
fi
popd
