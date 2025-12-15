#!/bin/bash
# setup
export AEROLAB_HOME=$(pwd)/bob/home
export AEROLAB_TEST=1
export AEROLAB_TELEMETRY_DISABLE=1
AL=./aerolab
rm -rf bob
mkdir bob

# prep
./build.sh
$AL config backend -t docker --inventory-cache
$AL config defaults -k '*.FeaturesFilePath' -v /Users/rglonek/aerolab/features/
$AL version

# cleanup
$AL inventory delete-project-resources -f

# create 2 clusters
$AL cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' -n dc1
$AL cluster create -c 2 -d ubuntu -i 24.04 -v '8.*' -n dc2

# test tls
$AL tls generate -n dc1
$AL tls copy -s dc1 -d dc2

# test xdr
$AL xdr connect -S dc1 -D dc2
$AL cluster destroy -f -n dc1,dc2
$AL xdr create-clusters -n dc1 -N dc2,dc3 -c 2 -C 1 -v '8.*' --privileged

# test data
$AL data insert -n dc1 -a 1 -z 10000
$AL data delete -n dc1 -a 1 -z 3000
$AL attach shell -n dc2 -- asadm -e info
$AL attach shell -n dc3 -- asadm -e info

# test net iptables
$AL net block -s dc1 -d dc3
$AL net list
$AL data insert -n dc1 -a 1 -z 3000
$AL attach shell -n dc3 -- asadm -e info
$AL net unblock -s dc1 -d dc3

# test net easytc
$AL net loss-delay -s dc1 -d dc2 -a set -D 200 -L 10 -o -p 3000 -v || $AL attach shell -n dc2 -- cat /tmp/runtc.sh
$AL cluster destroy -f -n dc1,dc2,dc3

# test clients
$AL cluster create -v '8.*'
$AL client create none -n none
$AL client create base -n base
$AL client create vscode -n vscode
$AL client create ams -n ams
$AL client configure ams -n ams -s mydc
$AL client create tools -n tools
$AL client configure tools -n tools -m ams
$AL client create eksctl -n eksctl -r us-west-2 -k USER -s PASS -f /Users/rglonek/aerolab/features/features.conf.v2
$AL client create graph -n graph
$AL client list
for i in base none tools vscode ams eksctl graph; do $AL client destroy -f -n $i || echo "$i already does not exist"; done
$AL client list
