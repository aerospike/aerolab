#!/bin/bash

function runtest {
    set -e
    local backend=$1
    local invcache=$2
    local invcache_flag=""
    if [ "$invcache" == "true" ]; then
        invcache_flag="--inventory-cache"
    fi
    echo "🔧 Setting up test environment"
    rm -rf bob
    AL=./aerolab
    mkdir bob
    AEROLAB_HOME=$(pwd)/bob/home
    export AEROLAB_HOME
    export AEROLAB_TEST=1
    export AEROLAB_TELEMETRY_DISABLE=1

    # set backend and defaults for testing
    echo "🔧 Setting backend and defaults for testing"
    $AL config backend -t $backend $invcache_flag
    $AL config defaults -k '*.FeaturesFilePath' -v /Users/rglonek/aerolab/features/
    $AL config defaults -k '*.FeaturesFilePath' |grep rglonek

    # version
    echo "🔧 Checking version"
    $AL version

    # config commands
    echo "🔧 Running config commands"
    if [ "$backend" == "docker" ]; then
        $AL config docker list-networks
        $AL config docker prune-networks
    fi

    # showcommands commands
    echo "🔧 Running showcommands commands"
    $AL showcommands -d bob
    if [ $(ls bob |grep show |wc -l) -ne 3 ]; then
        echo "showcommands commands failed"
        exit 1
    fi

    # completion
    echo "🔧 Running completion"
    $AL completion bash -n > bob/completion.bash

    # installer
    echo "🔧 Running installer"
    $AL installer list-versions
    pushd bob
    ../$AL installer download -d ubuntu -i 24.04 -v '8.1.0.1'
    popd

    # cleanup
    echo "🔧 Running cleanup"
    $AL inventory delete-project-resources -f

    # templates
    echo "🔧 Running templates"
    $AL template vacuum
    $AL template list
    $AL template create --distro=ubuntu --distro-version=24.04 --aerospike-version='7.*' --arch=amd64 --owner=bob --no-vacuum

    # cluster create from template
    echo "🔧 Running cluster create from template"
    $AL cluster create -c 2 -d ubuntu -i 24.04 -v '7.*'

    # cluster grow - non-existing template
    echo "🔧 Running cluster grow - non-existing template"
    $AL cluster grow -c 2 -d ubuntu -i 24.04 -v '8.*'

    # cluster apply - grow
    echo "🔧 Running cluster apply - grow"
    $AL cluster apply -c 5 -d ubuntu -i 24.04 -v '8.*'

    # cluster apply - shrink
    echo "🔧 Running cluster apply - shrink"
    $AL cluster apply -c 4 -d ubuntu -i 24.04 -v '8.*' --force

    # cluster list
    echo "🔧 Running cluster list"
    $AL cluster list

    # cluster stop
    echo "🔧 Running cluster stop"
    $AL cluster stop

    # cluster start
    echo "🔧 Running cluster start"
    $AL cluster start

    # cluster stop - partial
    echo "🔧 Running cluster stop - partial"
    $AL cluster stop -n mydc -l 1-2

    # cluster start - full
    echo "🔧 Running cluster start - full"
    $AL cluster start -n mydc

    # aerospike stop
    echo "🔧 Running aerospike stop"
    $AL aerospike stop

    # aerospike start
    echo "🔧 Running aerospike start"
    $AL aerospike start

    # aerospike is-stable wait
    echo "🔧 Running aerospike is-stable wait"
    $AL aerospike is-stable -w -o 30 -i

    # aerospike status
    echo "🔧 Running aerospike status"
    $AL aerospike status

    # aerospike stop
    echo "🔧 Running aerospike stop"
    $AL aerospike stop -n mydc

    # aerospike upgrade
    echo "🔧 Running aerospike upgrade"
    $AL aerospike upgrade -n mydc -l 1-2 -v '8.*'

    # aerospike cold-start
    echo "🔧 Running aerospike cold-start"
    $AL aerospike cold-start -n mydc

    # cluster add exporter
    echo "🔧 Running cluster add exporter"
    $AL cluster add exporter

    # cluster add aerolab
    echo "🔧 Running cluster add aerolab"
    $AL cluster add aerolab

    # cluster attach - multiple versions
    echo "🔧 Running cluster attach - multiple versions"
    $AL cluster attach -l all -- ls /tmp
    $AL cluster attach -l all --parallel -- ls /tmp

    # attach shell - check that works too, and attach aql,asinfo,asadm
    echo "🔧 Running attach shell - check that works too, and attach aql,asinfo,asadm"
    $AL attach shell -l all -- ls /tmp
    $AL attach asadm -- -e info
    $AL attach aql -- -c 'show namespaces'

    # conf rack-id
    echo "🔧 Running conf rack-id"
    $AL conf rackid -l 1-2 -i 1
    $AL conf rackid -l 3-4 -i 2

    # conf sc
    echo "🔧 Running conf sc"
    $AL conf sc -r 2 -v

    # conf fix-mesh
    echo "🔧 Running conf fix-mesh"
    $AL conf fix-mesh

    # conf adjust
    echo "🔧 Running conf adjust"
    $AL conf adjust set network.heartbeat.interval 250

    # aerospike restart
    echo "🔧 Running aerospike restart"
    $AL aerospike restart -n mydc

    # aerospike is-stable wait
    echo "🔧 Running aerospike is-stable wait"
    $AL aerospike is-stable -w -o 30 -i

    # roster apply, show
    echo "🔧 Running roster apply, show"
    $AL roster apply -n mydc
    $AL roster show -n mydc

    # files upload, download, sync
    echo "🔧 Running files upload, download, sync"
    touch bob/test.txt
    $AL files upload -n mydc bob/test.txt /tmp/test.txt
    $AL files download -n mydc /tmp/test.txt bob/dlout
    $AL files sync -n mydc -l 1 /tmp/test.txt

    # logs get
    echo "🔧 Running logs get"
    $AL logs get -n mydc -j -d bob/logsget

    # logs show
    echo "🔧 Running logs show"
    $AL logs show -n mydc -j

    # inventory list, ansible, genders, hostfile
    echo "🔧 Running inventory list, ansible, genders, hostfile"
    $AL inventory list
    $AL inventory ansible
    $AL inventory genders
    $AL inventory hostfile

    # cluster destroy
    echo "🔧 Running cluster destroy"
    $AL cluster destroy -n mydc --force

    # template destroy - list the 7x templates, find the right one and destroy it
    echo "🔧 Running template destroy"
    tmpl=$($AL template list -o tsv |awk '{print $9" "$10}' |grep aerospike |grep ' 7.' |awk '{print $2}' |cut -d'-' -f 1)
    $AL template destroy --distro=ubuntu --distro-version=24.04 --aerospike-version=$tmpl --arch=amd64 --force
    $AL inventory list
}

function runostest {
    set -e
    local backend=$1
    local invcache=$2
    local invcache_flag=""
    if [ "$invcache" == "true" ]; then
        invcache_flag="--inventory-cache"
    fi
    echo "🔧 Setting up test environment"
    rm -rf bob
    AL=./aerolab
    mkdir bob
    AEROLAB_HOME=$(pwd)/bob/home
    export AEROLAB_HOME
    export AEROLAB_TEST=1
    export AEROLAB_TELEMETRY_DISABLE=1

    # set backend and defaults for testing
    echo "🔧 Setting backend and defaults for testing"
    $AL config backend -t $backend $invcache_flag
    $AL config defaults -k '*.FeaturesFilePath' -v /Users/rglonek/aerolab/features/
    $AL config defaults -k '*.FeaturesFilePath' |grep rglonek

    # version
    echo "🔧 Checking version"
    $AL version

    # cleanup
    echo "🔧 Running cleanup"
    $AL inventory delete-project-resources -f

    # test all OS and their versions
    echo "🔧 Deploying ubuntu 24.04"
    $AL cluster create -n mydc -c 1 -d ubuntu -i 24.04 -v '8.*'
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying ubuntu 22.04"
    $AL cluster create -n mydc -c 1 -d ubuntu -i 22.04 -v '8.*'
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying ubuntu 20.04"
    $AL cluster create -n mydc -c 1 -d ubuntu -i 20.04 -v '8.*'
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying centos 9"
    $AL cluster create -n mydc -c 1 -d centos -i 9 -v '8.*'
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying centos 8"
    $AL cluster create -n mydc -c 1 -d centos -i 8 -v '8.*'
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying rocky 9"
    $AL cluster create -n mydc -c 1 -d rocky -i 9 -v '8.*'
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying rocky 8"
    $AL cluster create -n mydc -c 1 -d rocky -i 8 -v '8.*'
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying debian 12"
    $AL cluster create -n mydc -c 1 -d debian -i 12 -v '8.*'
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying debian 11"
    $AL cluster create -n mydc -c 1 -d debian -i 11 -v '8.*'
    $AL cluster destroy -n mydc --force
    echo "🔧 Done testing OS:Version combinations"

    # cleanup
    echo "🔧 Running cleanup"
    $AL inventory delete-project-resources -f
}

set -e
[ -f ./aerolab ] || ./build.sh
rm -rf test-results
mkdir -p test-results
#backends=("docker" "aws" "gcp")
#invcaches=("true" "false")
backends=("docker")
invcaches=("false" "true")
for backend in "${backends[@]}"; do
    for invcache in "${invcaches[@]}"; do
        echo "🧪 Testing $backend with inventory cache $invcache"
        runtest $backend $invcache > test-results/$backend-$invcache.log 2>&1
    done
    echo "🧪 Testing OS support on $backend"
    runostest $backend false > test-results/$backend-os.log 2>&1
done
echo "✅ Done testing all backends and inventory caches"

# TODO test working with one backend and then switching to next and then switching to previous, using all different flags
