#!/bin/bash

set -e
if [ "$1" = "all" ]; then
  echo "$(date) Running go generate..."
  pushd ../../cli/
  go generate ./...
  popd
fi
echo "$(date) Building aerolab..."
rm -rf ./aerolab
go build -o ./aerolab ../../cli/.
echo "$(date) Aerolab built successfully"
