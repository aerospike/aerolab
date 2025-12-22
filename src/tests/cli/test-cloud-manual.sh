#!/bin/bash
set -ex
./aerolab config backend -t aws -r ca-central-1
./aerolab cloud list-instance-types

# check correct route and other settings
./aerolab cloud clusters create -n bobtest -i m5d.xlarge -r us-west-2 --availability-zone-count=2 --cluster-size=2 --data-storage=memory --credentials=bobuser:bobpass1 --dry-run

# actually create cluster
./aerolab cloud clusters create -n bobtest -i m5d.xlarge -r us-west-2 --availability-zone-count=2 --cluster-size=2 --data-storage=memory --credentials=bobuser:bobpass1

# check route updates to second route
./aerolab cloud clusters create -n bobtest2 -i m5d.xlarge -r us-west-2 --availability-zone-count=2 --cluster-size=2 --data-storage=memory --credentials=bobuser:bobpass1 --dry-run

# check pretty print
./aerolab cloud clusters list

# check output in detail
./aerolab cloud clusters list -o jq --pager

# get DBID
DBID=$(./aerolab cloud clusters list -o json | jq -r '.clusters[] | select(.name == "bobtest") | .id' | head)

# run in another terminal to test the wait command
./aerolab cloud clusters wait -i $DBID -s provisioning
./aerolab cloud clusters wait -i $DBID --status-ne=provisioning

# check route still updates to second route, after cluster exists
./aerolab cloud clusters create -n bobtest2 -i m5d.xlarge -r us-west-2 --availability-zone-count=2 --cluster-size=2 --data-storage=memory --credentials=bobuser:bobpass1 --dry-run

# check credentials exist
./aerolab cloud clusters credentials list -c $DBID

# get hostid and certs
./aerolab cloud clusters get host -n bobtest
./aerolab cloud clusters get tls-cert -n bobtest

# add credentials
./aerolab cloud clusters credentials create -c $DBID -u bobadmin -p bobadmin -r read-write --wait

# list credentials - get CID
CREDID=$(./aerolab cloud clusters credentials list -c $DBID 2>/dev/null | jq -r '.credentials[] | select(.name == "bobadmin") | .id')

# remove credentials
./aerolab cloud clusters credentials delete -c $DBID -C $CREDID

# check credentials exist
./aerolab cloud clusters credentials list -c $DBID

# destroy cluster with wait
./aerolab cloud clusters delete -c $DBID -w -f

# list clusters
./aerolab cloud clusters list
