#!/bin/bash

## source this file to install golang and set $PATH in running shell

GOVERSION="{{.GoVersion}}"
if ! command -v curl &> /dev/null; then
    if command -v apt &> /dev/null; then
        apt-get update
        apt-get install -y curl || exit 1
    elif command -v yum &> /dev/null; then
        yum install -y curl || exit 1
    else
        echo "Neither apt nor yum found. Cannot install curl."
        exit 1
    fi
fi

ARCH=amd64
if [ "$(uname -m)" == "aarch64" ] || [ "$(uname -m)" == "arm64" ]; then
    ARCH=arm64
fi

set -e
curl -L -o ${GOVERSION}.linux-${ARCH}.tar.gz https://go.dev/dl/${GOVERSION}.linux-${ARCH}.tar.gz
rm -rf /usr/local/go && tar -C /usr/local -xzf ${GOVERSION}.linux-${ARCH}.tar.gz
set +e

# install in profile.d
if [ -d /etc/profile.d ]; then
    if [ ! -f /etc/profile.d/99-golang.sh ]; then
        cat <<'EOF' > /etc/profile.d/99-golang.sh
if [[ ":$PATH:" != *":/usr/local/go/bin:"* ]]; then
    export PATH=$PATH:/usr/local/go/bin
fi
EOF
        chmod +x /etc/profile.d/99-golang.sh
    fi
fi

# install in current user's shellrc
F="$HOME/.$(basename "$(readlink /proc/$$/exe)")rc"
if [ -f "$F" ]; then
    if ! grep -q ":/usr/local/go/bin:" "$F"; then
        cat <<'EOF' >> "$F"
if [[ ":$PATH:" != *":/usr/local/go/bin:"* ]]; then
    export PATH=$PATH:/usr/local/go/bin
fi
EOF
    fi
fi

# add to current user's shellrc
if [[ ":$PATH:" != *":/usr/local/go/bin:"* ]]; then
    export PATH=$PATH:/usr/local/go/bin
fi
