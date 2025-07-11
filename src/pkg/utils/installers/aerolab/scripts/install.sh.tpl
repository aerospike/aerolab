# shellcheck disable=SC2148
DL_URL_ARM64="{{.DownloadURLARM64}}"
DL_URL_AMD64="{{.DownloadURLAMD64}}"

set -e

if [ "$(uname -m)" == "aarch64" ] || [ "$(uname -m)" == "arm64" ]; then
    DL_URL="$DL_URL_ARM64"
else
    DL_URL="$DL_URL_AMD64"
fi

curl -L -o /tmp/aerolab.zip "$DL_URL"

pushd /tmp
unzip aerolab.zip
mv aerolab /usr/local/bin/aerolab
chmod +x /usr/local/bin/aerolab
popd

rm -f /tmp/aerolab.zip

set +e
