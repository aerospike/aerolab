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

if command -v dnf &> /dev/null; then
    dnf install -y "$FILE_NAME" || exit 1
elif command -v yum &> /dev/null; then
    yum -y localinstall "$FILE_NAME" || exit 1
else
    rpm -Uvh --replacepkgs "$FILE_NAME" || exit 1
fi

if ! command -v asd &> /dev/null; then
    echo "Aerospike server is not installed after attempting to install"
    exit 1
fi
