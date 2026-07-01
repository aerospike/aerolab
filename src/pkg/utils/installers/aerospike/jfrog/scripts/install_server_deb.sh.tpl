# shellcheck disable=SC2148
UPGRADE="{{.Upgrade}}"
if [ "$UPGRADE" != "true" ]; then
    if command -v asd &> /dev/null; then
        echo "Aerospike server is already installed"
        exit 0
    fi
fi

FILE_NAME="{{.FileName}}"

if [ ! -f "$FILE_NAME" ]; then
    echo "Package file $FILE_NAME not found"
    exit 1
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update || true

if ! apt-get install -y --allow-downgrades "$FILE_NAME"; then
    dpkg -i "$FILE_NAME" || true
    apt-get -f install -y || exit 1
fi

if ! command -v asd &> /dev/null; then
    echo "Aerospike server is not installed after attempting to install"
    exit 1
fi
