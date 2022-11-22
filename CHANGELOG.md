#### FUTURE
* Add support for exporter on `arm`
* Test all `arm` features on clients

#### 4.3.9
* aws backend arm template creation bugfix
* bugfix: aerospikeGetUrl would incorrectly assume version is provided in a not-required early arm version check
* `client tools` arm support added
* add `-i` option to print parseable assigned IPs of cluster/client nodes in `cluster list` command
* update the following dependency libraries:
  * `github.com/aerospike/aerospike-client-go/v6 v6.6.0`
  * `github.com/aws/aws-sdk-go v1.44.143`
  * `golang.org/x/crypto v0.3.0`
  * `golang.org/x/term v0.2.0`
  * `golang.org/x/sys v0.2.0`

#### 4.3.8
* fix naming conventions

#### 4.3.7
* ams exporter fix aws installation
* add client attach option to detach from stdin (do not terminate node command on exit)
* make base install script retry once

#### 4.3.6
* small improvement in exporter installation procedure
* change exporter download URL to use artifacts download URLs

#### 4.3.5
* added option `template vacuum` to remove any leftover template containers/instances from failed template creation
* template vacuuming will auto-run if templating fails, unless `--no-vacuum` is specified
* update download URL to use download.aerospike.com
* disable node jupyter client due to compatibility issues
* add vscode client with java,go,python,dotnet sdks

#### 4.3.4
* improve shell parsing for `aws` backend for `attach shell -- ...`
* install best-practices script on `aws` backend when deploying clusters
  * thp, min_free_kbytes, swappiness
* bugfix: `data insert` functions used `rand.NewSource()` without thread safety
* bugfix: `cluster start` would not work on partial start (some nodes only) due to failure in `fixMesh` code N/A ip handling
* hide `client add` option, as most clients have a specific OS:Ver requirement, making this option more annoying than useful
* experimental `jupyter` client added with `go,python,java,node,dotnet` kernels and aerospike client libraries
* experimental `trino` client added
* fix `net loss-delay` in source being client bug
* adjust installer downloader to new 6.2 aerospike naming convention

#### 4.3.3
* bump version of all dependencies to latest
  * resolves a number of known issues in golang/x/crypto, golang/x/term and golang/x/sys

#### 4.3.2
* add support for pre-selected eu-central-1
* add ability for aws backend to automatically lookup AMIs for any region using DescribeImages
* add `rest-api` command to allow for using aerolab as a rest-api webserver (basic, not full rest-api) instead of cli interface
* error handling improvements
* minor flow bugfixes
* package `aerolab` as `pkg` for MacOS, `deb` for debian/ubuntu, `rpm` for rhel/centos and generic `zip` for linux

#### 4.3.1
* update aerospike client libraries to latest versions

#### 4.3.0
* New Features:
  * add `ams` client installation system
  * add option where `client start` and `cluster start` will execute all scripts under `/opt/autoload` to allow for 3rd-party script installations
  * add `client configure` option to allow for post-create configuration of certain clients, like `ams`
  * add `cluster add` option, to allow for adding extra features, for example `ams`
  * add `cluster add exporter` to install exporter in clusters (amd64 only for now)
* Improvements:
  * make `client attach` command and link it to `attach client`
* Bug fixes:
  * support installing StartScript in `client add tools`
  * fix "newclient.sh" for generating skel source files for new client development
  * `cluster start` with multiple clusters would not fix mesh config properly, nor start aerospike

#### 4.2.0
* New Features:
  * support arm deployments
* Improvements:
  * insert/delete data support running from client machine
  * make mesh mode default
* Bug fixes:
  * do not require features file on CE

#### 4.1.0
* Bug Fixes
  * fix documentation typos
  * fix zsh completion system
* New Features
  * add options to deploy client machines (aerolab client, aerolab attach client, backend support for server/client selector, files command support, tls command support)
    * clients: base, aerospike-tools
  * add client command documentation
* Improvements
  * `aerolab xdr connect` command: add support for cross-region aws backend

#### v4.0.2
* add nodeExpander:
  * (node list can now be: 1-10,15,-3 - i.e. 1 to 10, node 15, not node 3)
  * (or: ALL,-5 - i.e. all nodes except node 5)
* bugfix in ranges in `files sync`
* fix multiple completion bugs
* add useful print of config for tls generate command

#### v4.0.1
* add support for centos / rhel-based distros v 7 and 8 in aws
* add option to rename hostname of nodes to clusterName-nodeNo
* cleanup command line interface (changes cli usage)
* make-cluster distro version help page: remove centos 6 and add centos 8 and ubuntu 22.04 to supported values
* use submodules for common functions and methods
* remove support for centos 6 / rhel 6
* add 'edit' command line option for quick-editing a single file
* add 'sync' option for quick-syncing of files/directories from one node to other nodes
* make 'download' and 'upload' work on whole directories, recursively, not just files
* update readme and help pages
* update scripts/
* make help pages work without verifying if backend is working
* install a script to run before aerospike starts and after it stops
* add basic troubleshooting tools to templates
* add json output support to template and cluster list
* bash completion and zsh completion

#### v3.1.2
* `make-cluster` and `cluster-grow`: automatically add `cluster-name` to `aerospike.conf` unless specified not to by the `-O` switch
* remove dependency on obsolete `ioutil` package
* bump to latest `golang` version for compile
* cgo minimum macos version is locked during build
* small improvements in build and test scripts

#### v3.1.1
* run `conf-fix-mesh` automatically on `cluster-start`
* run `start-aerospike` on `cluster-start`
* make `conf-fix-mesh` work on partially-up clusters

#### v3.1.0
* fix `make-xdr-clusters` to support v6 of Aerospike
* fix bug in `cluster-grow` re discovery of versions
* fix bug in `cluster-grow` re installation on non-ubuntu script
* add check in `cluster-grow` and `make-cluster` to confirm that distro version is selected if distro isn't ubuntu
* add early check in `cluster-grow` and `make-cluster` - if requested version does not exist, error early, with a meaningful message
* add the `make-cluster -v 5.7.*` version lookup option information to help pages
* store deployed aerospike version in `/opt/aerolab.aerospike.version`
* deprecate `-5` switch for version selection in `xdr-connect`
* add `xdr-version` selector in `xdr-connect`, add `auto` option for auto-discovery
* add `restart-source` option in `xdr-connect` with default of 'yes' to allow for auto-restarting of source on xdr static configuration
* bring version discovery features from `make-cluster` into `upgrade-aerospike`

#### v3.0.4
* documentation cleanup
* fix version discovery mechanism
* add option for specifying partial version, for example `-v '4.9.0.*'` will find and use latest `4.9.0.` version
* added command `list-versions` to quickly lookup Aerospike versions, with switches for easy sorting and filtering, see `list-versions help` for more details

#### v3.0.3
* bugfix: aws backend using ubuntu 20.04 image correctly now
* aws backend make "waiting for node to come up" messages more explicit

#### v3.0.2
* add option in insert-data and delete-data to choose aerospike library version (4|5)

#### v3.0.1
* deploy-container move to ubuntu 20.04
* add basic tools to aerospike server containers
* set basic ubuntu version to 20.04
* update version discovery algorythm to allow for new naming conventions
* add aws backend ubuntu 20.04 discovery options
* comment out paxos-single-replica-limit in conf files (obsolete as of v6 of Aerospike)
* fix apt unattended install requirement
* fix dpkg force confold for unattended upgrades
* change gen-tls-certs to use 2048-bit keys

#### v3.0.0
* remove lxc backend
* code cleanup, lint
* move to semantic versioning
* add helper scripts
* cleanup documentation
* remove obsolete functions
* rename binaries
* move to go modules with versioning

#### 2.68
* fix osx-aio discovery mechanism

#### 2.67
* add auth mode external to insert/delete data
* add client warmup(100) to insert/delete data
* recompile with aerospike library v5

#### 2.66
* fix check in aws backend for public IP

#### 2.65
* fix tls - new requirements - cannot use Common Name any more

#### 2.64
* new aerospike website broke artifacts download links. This works around the problem.

#### 2.63
* satisfy libcurl4 dependency for asd 5.1+

#### 2.62
* improvement: will now check if instance in aws has public IP assigned before attempting to use the variable

#### 2.61
* insert-data now allows specifying to insert data only to X number of partitions and/or nodes, or to specify exact partition numbers to insert data to

#### 2.60
* fix clash in switches in net-loss-delay

#### 2.59
* error handling improvement: add handling of wrong docker image names
* updated templates to all have default-ttl 0
* udpated dependencies to latest version

#### 2.58
* fix support for running aerolab via symlinks or from a PATH env var

#### 2.57
* insert-data now supports specifying write policy (insert only, update, update_only, replace, replace_only)
* new option: delete-data, including durable delete
* packet net-loss-delay function now allows to specify -D to implement rules on destination
* net-block now allows to specify probability (partial packet loss, without EPERM issues if used on INPUT - destination)

#### 2.56
* add option to specify TTL in insert-data function

#### 2.55
* fix multicast mode deployments (broke in 2.53)

#### 2.54
* make cluster-name option inclusive, not exclusive

#### 2.53
* add option to automatically add cluster-name to aerospike.conf
* add support for aerospike 5.1+

#### 2.52.1
* fix fox issues with 2.52

#### 2.52
* allow installation on ubuntu 20.04 and centos 8

#### 2.51
* added more packages to auto-preinstall for deploy-container

#### 2.50
* deprecated deploy-amc
* new feature: deploy-container

#### 2.49
* added 'latest' recognition fix for aerospike version 5
* insert-data multithread switch fix
* add OS discovery for aerospike 5+
* fix links in aerolab downloads
* NEW: xdr-connect supports xdr in asd v5+, using the '-1' switch

#### 2.48
* fixed -u switch in insert-data

#### 2.47
* fixed setting correct writepolicy on aerospike insertData

#### 2.46
* minor bugfixes
* shruink binary size to 12MB from 56MB

#### 2.45
* AWS BACKEND FIX - small bug when dealing with IP address of nodes

#### 2.44
* attempt to install python3-setuptools as well as python3-distutils - because ubuntu loves making changes
* updated dependency versions and aerospike library to latest

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
* Initial stable golang release
