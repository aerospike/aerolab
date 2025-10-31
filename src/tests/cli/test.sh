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
    if [ "$backend" == "docker" ]; then
        $AL config backend -t $backend $invcache_flag
    elif [ "$backend" == "aws" ]; then
        $AL config backend -t $backend -r us-east-1 -P eks $invcache_flag
    fi
    $AL config defaults -k '*.FeaturesFilePath' -v /Users/rglonek/aerolab/features/
    $AL config defaults -k '*.FeaturesFilePath' |grep rglonek

    # version
    echo "🔧 Checking version"
    $AL version

    # cleanup
    echo "🔧 Running cleanup"
    if [ "$backend" == "docker" ]; then
        $AL inventory delete-project-resources -f
    else
        $AL inventory delete-project-resources -f --expiry
    fi

    # config commands
    echo "🔧 Running config commands"
    if [ "$backend" == "docker" ]; then
        $AL config docker list-networks
        $AL config docker prune-networks
    elif [ "$backend" == "aws" ]; then
        $AL config aws list-subnets
        $AL config aws list-security-groups
        $AL config aws create-security-groups -n bob-test-fw -p 3000-3005
        $AL config aws list-security-groups
        $AL config aws lock-security-groups -n bob-test-fw
        $AL config aws list-security-groups
        $AL config aws delete-security-groups -n bob-test-fw
        $AL config aws list-security-groups
        $AL config aws expiry-remove || true
        $AL config aws expiry-install
        $AL config aws expiry-list
        $AL config aws expiry-run-frequency -f 20
        $AL config aws expiry-list
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

    # templates
    echo "🔧 Running templates"
    $AL template vacuum
    $AL template list
    $AL template create --distro=ubuntu --distro-version=24.04 --aerospike-version='7.*' --arch=amd64 --owner=bob --no-vacuum

    # cluster create from template
    echo "🔧 Running cluster create from template"
    $AL cluster create -c 2 -d ubuntu -i 24.04 -v '7.*' -I t3a.xlarge --aws-disk type=gp2,size=20 --aws-disk type=gp2,size=30,count=3 --aws-expire=8h

    echo "🔧 Running config aws expiry-remove"
    $AL config aws expiry-remove

    # cluster grow - non-existing template
    echo "🔧 Running cluster grow - non-existing template"
    $AL cluster grow -c 2 -d ubuntu -i 24.04 -v '8.*' -I t3a.xlarge --aws-disk type=gp2,size=20 --aws-disk type=gp2,size=30,count=3 --aws-expire=8h

    # cluster apply - grow
    echo "🔧 Running cluster apply - grow"
    $AL cluster apply -c 5 -d ubuntu -i 24.04 -v '8.*' -I t3a.xlarge --aws-disk type=gp2,size=20 --aws-disk type=gp2,size=30,count=3 --aws-expire=8h

    # cluster apply - shrink
    echo "🔧 Running cluster apply - shrink"
    $AL cluster apply -c 4 -d ubuntu -i 24.04 -v '8.*' --force -I t3a.xlarge --aws-disk type=gp2,size=20 --aws-disk type=gp2,size=30,count=3 --aws-expire=8h

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

    if [ "$backend" == "aws" ]; then
        echo "🔧 Running conf namespace-memory"
        $AL conf namespace-memory -n mydc

        echo "🔧 Running inventory instance-types"
        $AL inventory instance-types

        #conf aws create-security-groups
        echo "🔧 Running conf aws create-security-groups"
        $AL config aws create-security-groups -n bob-test-fw -p 3000-3005

        #add firewall
        echo "🔧 Running add firewall"
        $AL cluster add firewall -n mydc -f bob-test-fw

        #add public-ip
        echo "🔧 Running add public-ip"
        $AL cluster add public-ip -n mydc

        #cluster partition create -p 16,16,16,16,16,16
        echo "🔧 Running cluster partition create"
        $AL cluster partition create -p 16,16,16,16,16,16
        $AL cluster partition list -n mydc

        #cluster partition conf --namespace=test --configure=device --filter-partitions=3-6
        echo "🔧 Running cluster partition conf"
        $AL cluster partition conf --namespace=test --configure=device --filter-partitions=3-6

        #cluster partition mkfs --filter-partitions=1,2
        echo "🔧 Running cluster partition mkfs"
        $AL cluster partition mkfs --filter-partitions=1,2

        #cluster partition conf --namespace=test --configure=pi-flash --filter-partitions=1
        echo "🔧 Running cluster partition conf"
        $AL cluster partition conf --namespace=test --configure=pi-flash --filter-partitions=1

        #cluster partition conf --namespace=test --configure=si-flash --filter-partitions=2
        echo "🔧 Running cluster partition conf"
        $AL cluster partition conf --namespace=test --configure=si-flash --filter-partitions=2

        #aerospike restart
        echo "🔧 Running aerospike restart"
        $AL attach shell -- cat /etc/fstab
        $AL attach shell -- cat /etc/aerospike/aerospike.conf
        $AL aerospike restart -n mydc
    fi

    # cluster destroy
    echo "🔧 Running cluster destroy"
    $AL cluster destroy -n mydc --force

    # template destroy - list the 7x templates, find the right one and destroy it
    echo "🔧 Running template destroy"
    if [ "$backend" == "docker" ]; then
        tmpl=$($AL template list -o tsv |awk '{print $9" "$10}' |grep aerospike |grep ' 7.' |awk '{print $2}' |cut -d'-' -f 1)
    else
        tmpl=$($AL template list -o tsv |awk '{print $10" "$11}' |grep aerospike |grep ' 7.' |awk '{print $2}' |cut -d'-' -f 1)
    fi
    $AL template destroy --distro=ubuntu --distro-version=24.04 --aerospike-version=$tmpl --arch=amd64 --force

    echo "🔧 Running inventory list"
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
    if [ "$backend" == "docker" ]; then
        $AL config backend -t $backend $invcache_flag
    elif [ "$backend" == "aws" ]; then
        $AL config backend -t $backend -r us-east-1 -P eks $invcache_flag
    fi
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
    $AL cluster create -n mydc -c 1 -d ubuntu -i 24.04 -v '8.*' -I t3a.xlarge --aws-expire=8h
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying ubuntu 22.04"
    $AL cluster create -n mydc -c 1 -d ubuntu -i 22.04 -v '8.*' -I t3a.xlarge --aws-expire=8h
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying ubuntu 20.04"
    $AL cluster create -n mydc -c 1 -d ubuntu -i 20.04 -v '8.*' -I t3a.xlarge --aws-expire=8h
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying centos 9"
    $AL cluster create -n mydc -c 1 -d centos -i 9 -v '8.*' -I t3a.xlarge --aws-expire=8h
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying centos 8"
    $AL cluster create -n mydc -c 1 -d centos -i 8 -v '8.*' -I t3a.xlarge --aws-expire=8h
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying rocky 9"
    $AL cluster create -n mydc -c 1 -d rocky -i 9 -v '8.*' -I t3a.xlarge --aws-expire=8h
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying rocky 8"
    $AL cluster create -n mydc -c 1 -d rocky -i 8 -v '8.*' -I t3a.xlarge --aws-expire=8h
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying debian 12"
    $AL cluster create -n mydc -c 1 -d debian -i 12 -v '8.*' -I t3a.xlarge --aws-expire=8h
    $AL cluster destroy -n mydc --force
    echo "🔧 Deploying debian 11"
    $AL cluster create -n mydc -c 1 -d debian -i 11 -v '8.*' -I t3a.xlarge --aws-expire=8h
    $AL cluster destroy -n mydc --force
    echo "🔧 Done testing OS:Version combinations"

    # cleanup
    echo "🔧 Running cleanup"
    if [ "$backend" == "docker" ]; then
        $AL inventory delete-project-resources -f
    else
        $AL inventory delete-project-resources -f --expiry
    fi
}

function testcloud {
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
    $AL config backend -t $backend -r us-east-1 -P eks $invcache_flag
    $AL config defaults -k '*.FeaturesFilePath' -v /Users/rglonek/aerolab/features/
    $AL config defaults -k '*.FeaturesFilePath' |grep rglonek

    # version
    echo "🔧 Checking version"
    $AL version

    # cleanup
    echo "🔧 Running cleanup"
    $AL inventory delete-project-resources -f

    # TODO: test aerospike cloud
}

set -e
[ -f ./aerolab ] || ./build.sh
rm -rf test-results
mkdir -p test-results
#backends=("docker" "aws" "gcp")
#invcaches=("false" "true")
backends=("aws")
invcaches=("true")
for backend in "${backends[@]}"; do
    for invcache in "${invcaches[@]}"; do
        echo "🧪 Testing $backend with inventory cache $invcache"
        runtest $backend $invcache > test-results/$backend-$invcache.log 2>&1
    done
    echo "🧪 Testing OS support on $backend"
    runostest $backend false > test-results/$backend-os.log 2>&1
    if [ "$backend" == "aws" ]; then
        echo "🧪 Testing cloud support on $backend"
        testcloud $backend false > test-results/$backend-cloud.log 2>&1
    fi
done
echo "✅ Done testing all backends and inventory caches"

# TODO still run ostest on aws
# TODO add gcp
# TODO test working with one backend and then switching to next and then switching to previous, using all different flags
# TODO test arm
# TODO AWS/GCP:
#   volumes       Volume management (AWS EFS/GCP Volume only)
# TODO aws test cloud
