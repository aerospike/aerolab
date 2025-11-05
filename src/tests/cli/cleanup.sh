#!/bin/bash

set -e
AEROLAB_HOME=$(pwd)/bob/home
export AEROLAB_HOME
export AEROLAB_TEST=1
export AEROLAB_TELEMETRY_DISABLE=1
export AL=./aerolab
[ -f ./aerolab ] || ./build.sh
backends=("docker" "aws" "gcp")
for backend in "${backends[@]}"; do
    echo "🔧 Setting backend $backend for cleanup"
    rm -rf bob
    mkdir bob
    if [ "$backend" == "docker" ]; then
        $AL config backend -t $backend
    elif [ "$backend" == "aws" ]; then
        $AL config backend -t $backend -r us-east-1 -P eks
    elif [ "$backend" == "gcp" ]; then
        $AL config backend -t $backend -r us-central1 -o aerolab-test-project-1
    fi

    echo "🧪 Cleaning up $backend"
    EXP="--expiry"
    if [ "$backend" == "docker" ]; then
        EXP=""
    fi
    $AL inventory delete-project-resources -f $EXP
    if [ "$backend" == "aws" ]; then
        # cleanup cloud
        $AL cloud secrets list |jq -r '.secrets[] | select(.description=="aerolab") | .id' |while read line; do
            $AL cloud secrets delete --secret-id $line
        done
        $AL cloud databases list |jq -r '.databases[] | select(.name == "aerolabtest") | .id' |while read line; do
            $AL cloud databases delete --database-id $line
        done
    fi
done
echo "✅ Done cleaning up all backends"
