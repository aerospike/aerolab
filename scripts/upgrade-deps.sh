#!/bin/bash
set -e
pushd ../src
pushd pkg/backend
go get -u
go mod tidy
popd
pushd pkg/expiry
go get -u
go mod tidy
popd
pushd pkg/expiry/gcp
go get -u
go mod tidy
popd
bash new-expiry-version.sh
go generate ./...
popd
