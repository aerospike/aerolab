# shellcheck disable=SC2148
ARCH=amd64
if [ "$(uname -m)" = "aarch64" ] || [ "$(uname -m)" = "arm64" ]; then
    ARCH=arm64
fi

PLATFORM=$(uname -s)_$ARCH

curl -sLO "https://github.com/eksctl-io/eksctl/releases/latest/download/eksctl_$PLATFORM.tar.gz" || exit 1

# (Optional) Verify checksum
curl -sL "https://github.com/eksctl-io/eksctl/releases/latest/download/eksctl_checksums.txt" | grep $PLATFORM | sha256sum --check || exit 1

tar -xzf eksctl_$PLATFORM.tar.gz -C /tmp || exit 1
rm eksctl_$PLATFORM.tar.gz || exit 1
mv /tmp/eksctl /usr/local/bin || exit 1
chmod +x /usr/local/bin/eksctl || exit 1
eksctl version || exit 1
