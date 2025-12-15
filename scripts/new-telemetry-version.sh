#!/bin/bash
set -e
pushd ../src/cli/cmd/v1
N=$(( $(cat telemetryVersion.txt) + 1 ))
printf '%d' "$N" > telemetryVersion.txt
popd
