#!/bin/bash

set -e
echo "Building aerolab..."
rm -rf ./aerolab
go build -o ./aerolab ../../cli/.
echo "Aerolab built successfully"
