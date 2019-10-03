#### 2.43
* added message on conf-fix-mesh to inform user that aerospike must be restarted
* handle invalid command line parameters without crashing
* correctly identify latest version of aerospike/amc when asked to install latest

#### 2.42
* added -A to make-cluster, to allow for fix of access-address if using AWS backend

#### 2.41
* AWS support will now ignore instances in 'terminated' state, as it should

#### 2.40
* AWS auto-discovery of AMIs based on default region in ~/.aws/config

#### 2.39
* AWS updated AMIs

#### 2.38
* AWS backend - can now specify subnetID

#### 2.37
* fix issue with directory creation make-cluster

#### 2.36
* fix critical bug with centos deployments

#### 2.35
* fix critical bug in ubuntu deployment

#### 2.34
* OPS-3268 - fixed auto-discovery of versions after ordering change in artifact webserver

#### 2.33
* fix for python3-distutils dependency in old ubuntu versions

#### 2.32
* large number of small typo fixes for messaging and error reporting
* OPS-3222 - fix naming in help pages
* OPS-3237 - provide warning if aerospike 4.6+ is used and feature file not provided

#### 2.31
* fixed never-ending template builds for centos7 - disabled firewall control (net-loss and rate control) on centos builds

#### 2.30
* added AMC support for versions <4 (versions 4+ already work) to deploy-amc

#### 2.29
* fixed critical bug with version numbers in deploy-amc, cluster-grow, make-cluster, upgrade-aerospike

#### 2.28
* upgradeAerospike feature
* netLoss - net-loss-delay command allow to specify network latency on a node (delay) or packet loss to be introduced, or limit max link speed
* docs: TRAFFIC_CONTROL.md, ADVANCED_LOOP.md

#### 2.27
* numerous fixes to docker backend support for privileged containers

#### 2.26
* fixed bug in aerolab download, which would malform parts of downloaded file (docker exec pseudo-tty issue)

#### 2.25
* insert-data TLS support
* docker privileged run switch
* numerous fixes for aws support

#### 2.24
Fixes:
* obfuscated full dev path from panics
* added error handling to parse command line params function
* command line parameter parser now accepts bool parameters without value. No need for '-f 1', can just do '-f'
* node-attach now allows to run the same command across multiple nodes, examples:
  * node-attach -l all -- /some/command
  * node-attach -l 1,2 -- /some/command
  * NOTE: simply doing node-attach -l 2 - will attach to bash shell on that node, as it used to. Old behaviour has not been changed 

#### 2.23
Features:
* help now supports --full flag, without which it doesn't print config file parameter examples

#### 2.22
Fixes:
* enable-xdr true matches set(s)-enable-xdr=true, which causes addition bug
* help pages should be 'comma-separated', not 'space-separated'
* it says 16.06 in help instead of 16.04
* fix error 401 on enterprise version - show actual useful error
* choosing aerolab make-xdr-clusters -c 4 -a 4 -m test,bar -d ubuntu -i 16.04 -v 4.5.0.3 causes OS version error (-d switch naming collision. Using '-x' now for xdr dc names)

Features:
* cluster-start, cluster-stop, cluster-destroy now accept multiple cluster names as comma-separated values
* cluster-start, cluster-stop, cluster-destroy now accept 'all' to affect all existing clusters
* nuke-template now accepts 'all' in distro, version and aerospike version. Set all 3 to 'all' to nuke all templates
* insert-data now allows user:password
* new version bin/osx-aio released, contains osx binary with embedded linux binary (so that insert-data works without having to specify a linux binary, seamless now)

#### 2.21
* checkUbuntuAerospikeVersion fix unchecked errors

#### 2.20
* upload had unhandled errors, fixed

#### 2.19
* insert-data fixed another unreported error
* insert-data command help updated to make it more clear

#### 2.18
* aerospike python client fails on ubuntu 18.04 with 'undefined symbol: OPENSSL_sk_num' error. Downgraded all client libraries to use 16.04

#### 2.17
* insert-data fixed reporting bug which would report error even if no error was present if the insert was too fast
* insert-data some errors went unreported and were treated as success, fixed

#### 2.16
* get-logs should return .log file names
* cluster-list on docker now only prints containers associated with aerolab
* added ldap deployment script and documentation on how to configure aerospike to deploy ldap
* fixed \r location on download progress report

#### 2.15
* aws backend: make-client, net-block, net-unblock, net-list

#### 2.14
* aws backend support - experimental, feature yet not implemented: make-client, net-block, net-unblock, net-list
* data-insert set socketTimeout to 0, timeout to 5 seconds and maxRetries to 2 to improve speed and stability
* created help for how to use AWS plugin
* fix issue with cluster-grow (bug in cluster node list count)

#### 2.13
* data inserter (auto-insert) to fill cluster quickly with data, with '-u 1' multithreading, very aggressive and best suitable for inserts up to 200'000 records at a time 

#### 2.12
* resolved issue with overlayfs (overlay bug, disabled overlayfs, lxc works again)
* made docker default throughout

#### 2.11
* modification to conf file are SHOWING \r\n INSTEAD OF JUST \n
* encryption at rest - created documentation on how to do it, provided conf template
* get-logs (download-logs, to download all node logs)
* gen-tls-certs now uses /etc/aeropsike/ssl/{TLS_NAME}/... instead of /etc/aerospike/ssl/... - in preparation for use of multiple TLS certs
* added notes on multiple certificates to the MAKETLS.md documentation file
* copy-tls-certs - copy from one node to another (or cluster->cluster)
* fixed issue with chDir
* added binutils installation as default (will now provide addr2line by default)

#### 2.10
* make-cluster: auto-workout ubuntu version required to run that version of aerospike and use that if none specified (instead of trying 18.04 everywhere)
* make-cluster: check file dir paths in template config => create dirs on the fly in container if needed
* chDir (-W) - add option to specify download path for CA cert generation and for download of aerospike tarballs

#### 2.9
* fixed: cannot use underscore '_' in cluster/client names, as this results in a container that needs to be removed with 'docker' commands
* added check in docker init() - check not only if docker exists, but also if it's running

#### 2.8
* cpu-limit, ram-limit, swap-limit implemented for docker
* *-limit example documentation
* added net-tools and vim as installed by default in each contianer
* minor error message improvements

#### 2.7
* fixed: net-list does not format correctly on docker and is slow
* feature: deploy-amc
* feature: make-cluster single-node allows to expose ports to host system

#### 2.6
* format cluster-list on docker - add IP assignment information
* make-xdr-clusters used -o in 2 places, fixed, destination-node-count switch is now -a, not -o

#### 2.5
* bug with cluster name in gen-tls-certs function
* bug with aerospike start/stop script for docker ubuntu container (no systemd)
* minor bugs and issues

#### 2.4
* added feature: aerolab upload -n CLUSTER_NAME -l NODE_LIST -i INPUT_FILE -o OUTPUT_FILE
* added feature: aerolab download -n CLUSTER_NAME -l NODE_LIST -i INPUT_FILE -o OUTPUT_FILE
* NOTE: upload/download small files only, this is highly inefficient as it reads a whole file to RAM before saving it

#### 2.3
* In docker, auto-fix log location of aerospike log in aerospike.conf (no journalctl)
* If target (remote or local) is Darwin, use docker default, otherwise use LXC default
* Fixed Centos creation never finishes for template on centos7
* Fixed custom startup script for aerospike in centos7 on docker (damn you, docker!)

#### 2.2
* fix: dependency check for lxc on bionic, as package names changed :)
* PART OF: docker-ce docker functionality initial EXPERIMENTAL - see FUTURE.md for known bugs
* disabling btrfs as 18.04 has broken btrfs lxc-copy: https://github.com/lxc/lxc/issues/2612
* enabled overlayfs in lxc mode as workaround from btrfs lxc-copy issue

#### 2.1
* fix: nuke_template test and add stop before destroy
* fix: cluster-start hangs on exit if '-l' is specified. IP assignment wait fail

#### 2.0
* Initial stable release
