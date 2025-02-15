#!/bin/bash

rm -rf ../backend/expiry.linux.amd64.zip
touch ../backend/expiry.linux.amd64.zip
set -e
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o bootstrap -tags lambda.norpc .
rm -rf ../backend/expiry.linux.amd64.zip
zip -j ../backend/expiry.linux.amd64.zip bootstrap
rm -f bootstrap
