# shellcheck disable=SC2148
UPGRADE="{{.Upgrade}}"
if [ "$UPGRADE" != "true" ]; then
    if command -v aerospike-prometheus-exporter &> /dev/null; then
        echo "Aerospike Prometheus Exporter is already installed"
        exit 0
    fi
else
    # backup the ape configs if they exist
    rm -rf /etc/aerospike-prometheus-exporter.backup
    if [ -d /etc/aerospike-prometheus-exporter ]; then
        cp -a /etc/aerospike-prometheus-exporter /etc/aerospike-prometheus-exporter.backup
    fi
fi

FILE_NAME="{{.FileName}}"

# install
pushd / || exit 1
tar -C / -zxvf "$FILE_NAME" || exit 1
rm -rf .scripts
popd || exit 1

# check if it installed successfully
if ! command -v aerospike-prometheus-exporter &> /dev/null; then
    echo "Aerospike Prometheus Exporter is not installed after attempting to install"
    exit 1
fi

if [ "$UPGRADE" == "true" ]; then
    # restore the backup
    if [ -d /etc/aerospike-prometheus-exporter.backup ]; then
        rm -rf /etc/aerospike-prometheus-exporter.new
        mv /etc/aerospike-prometheus-exporter /etc/aerospike-prometheus-exporter.new
        mv /etc/aerospike-prometheus-exporter.backup /etc/aerospike-prometheus-exporter
    fi
fi

# done
mkdir -p /etc/systemd/system/multi-user.target.wants
if [ ! -L /etc/systemd/system/multi-user.target.wants/aerospike-prometheus-exporter.service ]; then
    ln -s /usr/lib/systemd/system/aerospike-prometheus-exporter.service /etc/systemd/system/multi-user.target.wants/aerospike-prometheus-exporter.service
fi
systemctl daemon-reload || echo "No systemctl, skipping daemon-reload"
