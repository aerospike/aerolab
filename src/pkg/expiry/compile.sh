#!/bin/bash

rm -rf ../backend/backends/expiry.linux.amd64.zip
touch ../backend/backends/expiry.linux.amd64.zip
set -e
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o ./gcp/bootstrap -tags lambda.norpc .
rm -rf ../backend/backends/expiry.linux.amd64.zip
cd gcp
zip -j ../../backend/backends/expiry.linux.amd64.zip bootstrap function.go
rm -f bootstrap
cd ..
