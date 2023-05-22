#!/bin/bash
comm=aerolab-next

$comm config backend -t gcp -o aerolab-test-project-2

function cleanup {
    $comm cluster destroy
    $comm cluster destroy -n source,dest
    $comm template destroy -d all -i all -v all
    $comm template vacuum
    $comm cluster list
    $comm template list
}

function templates {
    $comm template create --instance=e2-medium --zone=us-central1-a
    $comm template list
}

function firewall {
    echo "Cleanup"
    $comm config gcp delete-firewall-rules
    echo "Create"
    $comm config gcp create-firewall-rules
    echo "List"
    $comm config gcp list-firewall-rules
    echo "Lock"
    $comm config gcp lock-firewall-rules
    echo "List"
    $comm config gcp list-firewall-rules
}

function create_cluster {
    echo "Create default"
    $comm cluster create -v 6.3.0.3 -c 2 --instance e2-medium --disk=balanced:20 --disk=ssd:30 --external-ip --zone=us-central1-a
    echo "grow centos"
    $comm cluster grow -v 6.3.0.3 -c 2 --instance e2-medium --disk=balanced:20 --disk=ssd:30 --external-ip --zone=us-central1-a -d centos -i 8
    echo "grow debian"
    $comm cluster grow -v 6.3.0.3 -c 2 --instance e2-medium --disk=balanced:20 --disk=ssd:30 --external-ip --zone=us-central1-a -d debian -i 11
    echo "list"
    $comm cluster list
    echo "destroy 2 nodes"
    $comm cluster destroy -l 5,6
    echo "list"
    $comm cluster list
    echo "exporter"
    $comm cluster add exporter
    echo "cluster stop"
    $comm cluster stop -l 3,4
    echo "list"
    $comm cluster list
    echo "cluster start"
    $comm cluster start -l 3,4
    echo "list"
    $comm cluster list
}

function attach {
    $comm attach shell -l all -- asinfo -v service
    $comm attach asadm -- -e info
}

function asdcomm {
    $comm aerospike stop
    $comm attach shell -l all -- pidof asd
    $comm aerospike start
    sleep 5
    $comm attach shell -l all -- pidof asd
}

function fixmesh {
    $comm conf fix-mesh
    $comm attach shell -l all -- cat /etc/aerospike/aerospike.conf |grep mesh-seed-address-port
    $comm cluster list
}

function updown {
    echo "bob" > test.deleteme
    $comm files upload test.deleteme /root/test
    $comm attach shell -l all -- cat /root/test
    rm -f test.deleteme
    $comm files download /root/test test.deleteme
    cat test.deleteme
    rm -f test.deleteme
}

function roster {
    $comm roster show
    $comm roster apply
}

function xdr {
    $comm xdr create-clusters -n source -N dest -c 2 -C 2  --instance e2-medium --disk=balanced:20 --zone=us-central1-a
    $comm cluster list
    $comm data insert -n source
    $comm attach shell -n source -- asadm -e info
    $comm attach shell -n dest -- asadm -e info
}

function all {
    echo " <><> Cleanup <><>"
    cleanup
    echo "Press ENTER to continue"
    read

    echo " <><> Test templating <><>"
    templates
    echo "Press ENTER to continue"
    read

    echo " <><> Test firewall (config gcp .) <><>"
    firewall
    echo "Press ENTER to continue"
    read

    echo " <><> Test cluster commands <><>"
    create_cluster
    echo "Press ENTER to continue"
    read

    echo " <><> Test attach <><>"
    attach
    echo "Press ENTER to continue"
    read

    echo " <><> Test asd commands <><>"
    asdcomm
    echo "Press ENTER to continue"
    read

    echo " <><> Test fixmesh <><>"
    fixmesh
    echo "Press ENTER to continue"
    read

    echo " <><> Test upload download <><>"
    updown
    echo "Press ENTER to continue"
    read

    echo " <><> Test roster <><>"
    roster
    echo "Press ENTER to continue"
    read

    echo " <><> Test xdr <><>"
    xdr
    echo "Press ENTER to continue"
    read

    echo "Now manually test partitioner and clients and press ENTER to cleanup when done"
    echo "Press ENTER to continue"
    read
    clenaup
}

all
