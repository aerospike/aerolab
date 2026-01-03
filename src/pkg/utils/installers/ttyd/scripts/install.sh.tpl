# shellcheck disable=SC2148

DEST_PATH="{{.DestPath}}"
DOWNLOAD_URL_ARM64="{{.DownloadURLARM64}}"
DOWNLOAD_URL_AMD64="{{.DownloadURLAMD64}}"
ENABLE_TTYD="{{.EnableTtyd}}"
START_TTYD="{{.StartTtyd}}"

DOWNLOAD_URL=${DOWNLOAD_URL_AMD64}
if [ "$(uname -m)" == "aarch64" ] || [ "$(uname -m)" == "arm64" ]; then
    DOWNLOAD_URL=${DOWNLOAD_URL_ARM64}
fi

set -e
echo "Installing ttyd from ${DOWNLOAD_URL}..."
curl -L -o "${DEST_PATH}" "${DOWNLOAD_URL}"
chmod 0755 "${DEST_PATH}"

# Create systemd service file
cat <<'EOF' > /usr/lib/systemd/system/ttyd.service
[Unit]
Description=ttyd - Share your terminal over the web
After=network.target

[Service]
Type=simple
ExecStart=/usr/local/bin/ttyd -p 7681 -W bash
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

set +e
systemctl daemon-reload || true
if [ "$ENABLE_TTYD" == "true" ]; then
    systemctl enable ttyd || exit 1
fi
if [ "$START_TTYD" == "true" ]; then
    systemctl start ttyd || exit 1
fi

echo "ttyd installation completed"

