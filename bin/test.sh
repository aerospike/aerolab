aerolab="./aerolab-macos"
cluster="testcluster"

function handle() {
    ${1}
    if [ $? -ne 0 ]
    then
        echo "TEST FAILED: ${1}"
        exit 1
    else
        echo "TEST SUCCESS: ${1}"
    fi
}

function check_version() {
    ${aerolab} version
    return $?
}

function check_destroy() {
    ${aerolab} make-cluster -n ${cluster} -f
}

function check_make_basic() {
    ${aerolab} make-cluster -n ${cluster} -c 2 -m mesh
}

handle check_version
handle check_make_basic
handle check_destroy
