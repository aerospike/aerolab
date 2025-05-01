set -e
pushd pkg/backend/backends
N=$(( $(cat expiry.version.txt) + 1 ))
printf $N > expiry.version.txt
popd

