#!/bin/bash
# Increment the AGI version number
# This triggers automatic AGI template recreation when the version changes
set -e
pushd ../src/pkg/agi
N=$(( $(cat agi.version.txt) + 1 ))
printf '%d' "$N" > agi.version.txt
echo "AGI version incremented to: $N"
popd

