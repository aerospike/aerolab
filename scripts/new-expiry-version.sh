#!/bin/bash
set -e
pushd ../src/pkg/backend/backends
N=$(( $(cat expiry.version.txt) + 1 ))
printf $N > expiry.version.txt
popd
pushd ../src/pkg/expiry
bash compile.sh
popd
