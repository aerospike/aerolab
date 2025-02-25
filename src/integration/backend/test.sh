#!/bin/bash

export AWS_PROFILE=eks
export AEROLAB_AWS_TEST_REGIONS=ca-central-1

go test -v .
