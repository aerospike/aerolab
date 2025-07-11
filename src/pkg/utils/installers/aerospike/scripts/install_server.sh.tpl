# shellcheck disable=SC2148
UPGRADE="{{.Upgrade}}"
if [ "$UPGRADE" != "true" ]; then
    if command -v asd &> /dev/null; then
        echo "Aerospike server is already installed"
        exit 0
    fi
fi

FILE_NAME="{{.FileName}}"
fileDir=$(dirname "$FILE_NAME")
pushd "$fileDir" || exit 1

# get the top level directory name from the tar archive
topDir=$(tar -tzf "$FILE_NAME" | head -1 | cut -f1 -d"/")
if [ -z "$topDir" ]; then
    echo "Could not determine top directory name from tar archive"
    exit 1
fi

# extract the file
tar -xzf "$FILE_NAME" || exit 1

# check if the file exists
if [ ! -f "$topDir/asinstall" ]; then
    echo "File $topDir/asinstall not found after attempting to extract"
    exit 1
fi

# run the install script
pushd "$topDir" || exit 1
./asinstall || exit 1
popd || exit 1

# check if it installed successfully
if ! command -v asd &> /dev/null; then
    echo "Aerospike server is not installed after attempting to install"
    exit 1
fi

if ! command -v asinfo &> /dev/null; then
    echo "Aerospike server is not installed after attempting to install"
    exit 1
fi

# done
popd || exit 1
