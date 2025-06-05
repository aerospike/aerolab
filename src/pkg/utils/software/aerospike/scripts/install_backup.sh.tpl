# shellcheck disable=SC2148
UPGRADE="{{.Upgrade}}"
if [ "$UPGRADE" != "true" ]; then
    if command -v aerospike-backup-service &> /dev/null; then
        echo "Aerospike Backup Service is already installed"
        exit 0
    fi
fi

FILE_NAME="{{.FileName}}"

# cause we love workarounds
if ! command -v systemctl &> /dev/null; then
    touch /usr/bin/systemctl
    chmod +x /usr/bin/systemctl
fi

# install
if command -v yum &> /dev/null; then
    yum install -y "$FILE_NAME" || exit 1
elif command -v apt &> /dev/null; then
    apt install -y "$FILE_NAME" || exit 1
else
    echo "No package manager found, skipping install"
fi

# check if it installed successfully
if ! command -v aerospike-backup-service &> /dev/null; then
    echo "Aerospike Backup Service is not installed after attempting to install"
    exit 1
fi
