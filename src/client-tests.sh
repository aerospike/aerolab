#!/bin/bash

function check_client_create_add() {
    ${aerolab} client create base -n testcc -c 2 ${extra} || return 1
    ${aerolab} client grow tools -n testcc -c 2 ${extra} || return 1
    ${aerolab} client add tools -n testcc -l 1-2 || return 1
}

function check_client_list() {
    ${aerolab} client list || return 1
}

function check_client_attach() {
    ${aerolab} attach client -n testcc -l all -- /bin/bash -c "which aql" || return 1
}

function check_client_stop() {
    ${aerolab} client stop -n testcc || return 1
}

function check_client_start() {
    ${aerolab} client start -n testcc || return 1
}

function check_client_destroy() {
    ${aerolab} client destroy -f -n testcc || return 1
}

function check_client_files_download() {
    ${aerolab} files download -n testcc -c /etc/aerospike ./log/downloads/ || return 1
}

function check_client_files_upload() {
    ${aerolab} files upload -n testcc -l 2 -c build.sh /opt/bobbert || return 1
}

function check_client_files_sync() {
    ${aerolab} files sync -n testcc -l 2 -c -d testcc -C -p /opt/bobbert || return 1
    ${aerolab} attach client -n testcc -l all -- ls /opt/ || return 1
}

function check_client_files_edit() {
    ${aerolab} files edit -n testcc -c -l 1 -e ls /opt/ || return 1
}

function check_client_tls_generate() {
    ${aerolab} tls generate -n testcc -l 1 -C || return 1
}

function check_client_tls_copy() {
    ${aerolab} tls copy -s testcc -l 1 -c -d testcc -C
}

# TODO add NET TESTS
