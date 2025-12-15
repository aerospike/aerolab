#!/bin/bash

set -ex

# update main app
pushd ../src
go get -u ./...
go mod tidy
popd

# not doing this since expiry uses the main go.mod (should it really?)
#pushd ../src/pkg/expiry
#go get -u ./...
#go mod tidy
#popd

# update gcp expiry app since it's a separate module
pushd ../src/pkg/expiry/gcp
go get -u ./...
go mod tidy
popd

# update expiry version as we have updated the main app
bash new-expiry-version.sh

# regenerate the main app dependencies
pushd ../src/cli
go generate ./...
popd
