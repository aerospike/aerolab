#!/bin/bash

export AWS_PROFILE=eks
export AEROLAB_AWS_TEST_REGIONS=ca-central-1

if ! go test -v .; then
    echo "Test failed"
    exit 1
fi

echo "Test passed"
exit 0
