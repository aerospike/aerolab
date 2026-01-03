# shellcheck disable=SC2148

DEST_PATH="{{.DestPath}}"
DOWNLOAD_URL_ARM64="{{.DownloadURLARM64}}"
DOWNLOAD_URL_AMD64="{{.DownloadURLAMD64}}"
ENABLE_FILEBROWSER="{{.EnableFilebrowser}}"
START_FILEBROWSER="{{.StartFilebrowser}}"

DOWNLOAD_URL=${DOWNLOAD_URL_AMD64}
ARCH=amd64
if [ "$(uname -m)" == "aarch64" ] || [ "$(uname -m)" == "arm64" ]; then
    DOWNLOAD_URL=${DOWNLOAD_URL_ARM64}
    ARCH=arm64
fi

set -e
echo "Installing filebrowser from ${DOWNLOAD_URL}..."
pushd /tmp
curl -L -o filebrowser-linux-${ARCH}.tar.gz "${DOWNLOAD_URL}"
tar -zxvf filebrowser-linux-${ARCH}.tar.gz filebrowser
mv filebrowser "${DEST_PATH}"
chmod 0755 "${DEST_PATH}"
rm -f filebrowser-linux-${ARCH}.tar.gz
popd

# Create data directory
mkdir -p /var/lib/filebrowser

# Create systemd service file
cat <<'EOF' > /usr/lib/systemd/system/filebrowser.service
[Unit]
Description=Filebrowser - Web file manager
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/filebrowser -r / -a 0.0.0.0 -p 8080 --noauth -d /var/lib/filebrowser/filebrowser.db
Restart=always
RestartSec=5
WorkingDirectory=/var/lib/filebrowser

[Install]
WantedBy=multi-user.target
EOF

set +e
systemctl daemon-reload || true
if [ "$ENABLE_FILEBROWSER" == "true" ]; then
    systemctl enable filebrowser || exit 1
fi
if [ "$START_FILEBROWSER" == "true" ]; then
    systemctl start filebrowser || exit 1
fi

echo "filebrowser installation completed"

