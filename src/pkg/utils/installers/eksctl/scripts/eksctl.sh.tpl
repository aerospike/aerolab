# shellcheck disable=SC2148

# Retry helper function: tries command once, sleeps 1s, then retries once on failure
retry_cmd() {
    "$@" || { sleep 1; "$@"; }
}

ARCH=amd64
if [ "$(uname -m)" = "aarch64" ] || [ "$(uname -m)" = "arm64" ]; then
    ARCH=arm64
fi

PLATFORM=$(uname -s)_$ARCH

retry_cmd curl -sLO "https://github.com/eksctl-io/eksctl/releases/latest/download/eksctl_$PLATFORM.tar.gz" || exit 1

# (Optional) Verify checksum
retry_cmd curl -sL "https://github.com/eksctl-io/eksctl/releases/latest/download/eksctl_checksums.txt" | grep $PLATFORM | sha256sum --check || exit 1

tar -xzf eksctl_$PLATFORM.tar.gz -C /tmp || exit 1
rm eksctl_$PLATFORM.tar.gz || exit 1
mv /tmp/eksctl /usr/local/bin || exit 1
chmod +x /usr/local/bin/eksctl || exit 1
eksctl version || exit 1
