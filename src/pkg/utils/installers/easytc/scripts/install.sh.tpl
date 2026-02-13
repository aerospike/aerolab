# shellcheck disable=SC2148

# Retry helper: try once, sleep 1s, retry once
retry_cmd() {
    "$@" || { sleep 1; "$@"; }
}

DOWNLOAD_URL_ARM64="{{.DownloadURLARM64}}"
DOWNLOAD_URL_AMD64="{{.DownloadURLAMD64}}"

DOWNLOAD_URL=${DOWNLOAD_URL_AMD64}
ARCH=amd64
if [ "$(uname -m)" == "aarch64" ] || [ "$(uname -m)" == "arm64" ]; then
    DOWNLOAD_URL=${DOWNLOAD_URL_ARM64}
    ARCH=arm64
fi

set -e
pushd /tmp
retry_cmd curl -L -o easytc.${ARCH}.tgz ${DOWNLOAD_URL}
tar -zxvf easytc.${ARCH}.tgz
mv easytc /usr/local/bin/
set +e
