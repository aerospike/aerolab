#!/bin/bash

export GO111MODULE=on

# ensure we are in the right directory
if [ $(basename $(pwd)) != "src" ]; then
    cd src || exit 1
fi

# sanity check
mkdir -p ../bin || exit 1

# compile
MPATH=$(pwd)
env GOOS=linux GOARCH=amd64 go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o ../bin/aerolab-linux || exit 1
env GOOS=darwin GOARCH=amd64 CGO_CFLAGS="-mmacosx-version-min=11.3" CGO_LDFLAGS="-mmacosx-version-min=11.3" go build -gcflags=-trimpath=${MPATH} -asmflags=-trimpath=${MPATH} -ldflags="-s -w" -o ../bin/aerolab-macos || exit 1

# pack
cd ../bin || exit 1
upx aerolab-linux || exit 1
upx aerolab-macos || exit 1

# embed linux binary in macos binary for certain commands to work
echo -n ">>>>aerolab-osx-aio>>>>" >> aerolab-macos || exit 1
cat aerolab-linux >> aerolab-macos || exit 1
echo -n "<<<<aerolab-osx-aio" >> aerolab-macos || exit 1
