# shellcheck disable=SC2148

VERSION="{{.Version}}"
DOWNLOAD_URL_ARM64="{{.DownloadURLARM64}}"
DOWNLOAD_URL_AMD64="{{.DownloadURLAMD64}}"
ENABLE_GOPROXY="{{.EnableGoproxy}}"

DOWNLOAD_URL=${DOWNLOAD_URL_AMD64}
ARCH=amd64
if [ "$(uname -m)" == "aarch64" ] || [ "$(uname -m)" == "arm64" ]; then
    DOWNLOAD_URL=${DOWNLOAD_URL_ARM64}
    ARCH=arm64
fi

set -e
pushd /tmp
curl -L -o goproxy-${VERSION}.linux-${ARCH}.tar.gz ${DOWNLOAD_URL}
tar -zxvf goproxy-${VERSION}.linux-${ARCH}.tar.gz
mv goproxy /usr/local/bin/
mkdir -p /var/lib/goproxy
mkdir -p /etc/goproxy
cat <<'EOF' > /usr/lib/systemd/system/goproxy.service
[Unit]
Description=Goproxy
After=network.target
[Service]
Type=simple
ExecStart=/usr/local/bin/goproxy -config /etc/goproxy/config.yaml
Restart=always
WorkingDirectory=/var/lib/goproxy

[Install]
WantedBy=multi-user.target
EOF

set +e
systemctl daemon-reload || true
if [ "$ENABLE_GOPROXY" == "true" ]; then
    systemctl enable goproxy || exit 1
fi
