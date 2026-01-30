#!/bin/bash

set -ex

# if SKIP_UPGRADE is set, will just tidy and vendor all project, but will not upgrade any dependencies
SKIP_UPGRADE=0
[ "$1" == "--skip-upgrade" ] && SKIP_UPGRADE=1

# update main app
pushd ../src
[ "$SKIP_UPGRADE" -eq 0 ] && go get -u ./...
go mod tidy
GOWORK=off go mod vendor
popd

# not doing this since expiry uses the main go.mod (should it really?)
#pushd ../src/pkg/expiry
#go get -u ./...
#go mod tidy
#GOWORK=off go mod vendor
#popd

# update gcp expiry app since it's a separate module
pushd ../src/pkg/expiry/gcp
[ "$SKIP_UPGRADE" -eq 0 ] && go get -u ./...
go mod tidy
GOWORK=off go mod vendor
popd

# update expiry version as we have updated the main app
bash new-expiry-version.sh

# update agi version as we have updated the main app
bash new-agi-version.sh

# regenerate the main app dependencies
pushd ../src/cli
go generate ./...
popd
