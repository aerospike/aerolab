#!/bin/bash

export CGO_ENABLED=0

set -e
if [ "$1" = "all" ]; then
  echo "$(date) Running go generate..."
  pushd ../../cli/
  go generate ./...
  popd
fi
rm -rf ./aerolab ./aerolab-linux-amd64 ./aerolab-linux-arm64
echo "$(date) Building aerolab..."
GOOS=darwin GOARCH=amd64 go build -o ./aerolab -ldflags="-s -w" -trimpath ../../cli/.
echo "$(date) Building aerolab-linux-amd64..."
GOOS=linux GOARCH=amd64 go build -o ./aerolab-linux-amd64 -ldflags="-s -w" -trimpath ../../cli/.
upx ./aerolab-linux-amd64
echo "$(date) Building aerolab-linux-arm64..."
GOOS=linux GOARCH=arm64 go build -o ./aerolab-linux-arm64 -ldflags="-s -w" -trimpath ../../cli/.
upx ./aerolab-linux-arm64
echo "$(date) Aerolab built successfully"
