#### 7.2.0
* Allow specifying aerospike version number for agi.
* Check if locked security group / firewall rule does not include current IP. In this case, attempt to fix the rule.
* Add to aerolab config backend support for AWS profile names.
* New: `aerolab cluster share` - add a user’s public key option to instance (wrapper around `ssh-copy-id`).
* Expiries - add expiry information print during cluster creation.
* If expiry < 24h and attach is executed, print warning message (only on interactive shell, if `--` args are provided, do not print).
* AGI: Add notification support via https calls for status changes of AGI instances (finished ingest, sizing, logs too large, etc); with optional "fail on response error".
* Add global option `--beep` which will cause the terminal to issue a beep on aerolab exiting (finished/error task) - implements `CTRL+G` or `int(7)` terminal print.
  * The parameter can be specified multiple times. In this case, only one beep will be present on success, multiple beeps will be present on failure.
  * A `--beepf` is also added, which will only trigger beep on failures.
  * Useful for example in: `aerolab cluster destoy -f --beepf --beepf && aerolab cluster create --beep --beep` - beep twice on any failure, or once on successful completion of last command.
  * Beeps are globally configurable with environment variables `AEROLAB_BEEP=x` and `AEROLAB_BEEPF=x` where `x` is the number of beeps to apply.
* Inventory listing support `--aws-full` to print inventory in all AWS regions.
* AWS EFS volumes are supported (large cost saving and predictable IOPS on AWS platform, useful for AGI).
* TODO: Add spot instances option.
* TODO: Documentation (aws profile names, EFS support, spot instance support, notification support in agi).

#### 7.1.1
* GCP just made `DiscardLocalSsd` non-optional when stopping instances. Adjusting accordingly.

#### 7.1.0
* Update expiry system dependencies.
* Create a windows release.
* Upgrade - check if new version is available in the background, if so, inform user and point at the releases page.
* Telemetry - if telemetry is enabled, tag instances with telemetry tag so that the expiry system can report it.
* GCP backend: automatically attempt to enable cloud billing API for pricing lookups.
* Fix shell recovery after parallel attachment.
* Add AGI (Aerospike-Grafana-Integrated stack) for graphing Aerospike log statistics in Grafana.
* Add support for sindex flash in the partitioner.
* The `showcommands` command-set is now a part of aerolab.
* Recover the terminal from attach commands on error, not just on success.
* Make `.aerolab` directory only acessible by the user.
* Trim spaces and carriage returns from `deploy cluster` backend commands.
* The `list` commands now output in color by default, with pretty formatting and pagination.
* Aerospike version 7+ support added to `conf generate` and `cluster partition conf` commands.
* Added `lnav` tool for log exploring and parsing.

#### 7.0.1
* GitHub broke support for `wget` for source files in old repo releases page by producing `403 Forbidden` on `HEAD` requests; this fix implements a new link.
* Update dependencies to latest.

#### 7.0.0
* Message INFO at end of cluster create pointing user at AMS documentation informing them ams exists.
* Add deprecation warning around client machines and how they are handled in 8.0.
* Handle new AMS stack conventions and dashboards; auto-discover all available dashboards and folders.
* Add options for specifying replacement or additive dashboard lists to AMS stack.
* Add option to map and expose ports in Docker backend on a 1:1 pairing (eg 3100 node 1, 3101 node 2 etc) - so not workarounds are needed to access Aerospike clusters on Docker Desktop from the Desktop.
* Add MacOS packaging and signing to makefile to move fully away from bash scripts.
* Add `conf namespace-memory` option to configure memory for a namespace for a given RAM total percentage.
* Use GitHub Actions to create builds
* Setting `conf adjust` value params, if it's `..`, treat as literal dot.
* Option `conf adjust` now allows `get` to retrieve values.
* Support for deploying Amazon 2023 server >= 6.4.
* Support for deploying Debian 12 server >= 6.4.
* Create manual on using AWS Secrets Manager.
* Add option for auto-expiring clusters in GCP and AWS.

#### 6.2.0
* Add option to filter instance types by name.
* Add pricing information to `inventory instance-types` command.
* Sort options for `inventory instance-types` command.
* Add a 24-hour cache of `inventory instance-types` to allow for quick lookup.
* Add display of price information on cluster and client creation.
* Add option to only display price (without actually creating clusters or clients) using `--price` in the `create` commands.
* Add `nodes count` multiplier to `inventory instance-types` to allow for easy cost visualization.
* Track instance cost in instance tags/labels.
* Show instance running costs in `list` views.
* Parallelize the following commands (with thread limits):
  * `aerospike start/stop/restart/status`
  * `roster show/apply`
  * `files upload/download/sync`
  * `tls generate/copy`
  * `conf fix-mesh/rack-id`
  * `cluster create/start/partition */add exporter`
  * `xdr connect/create-clusters`
  * `client create base/none/tools`
  * `client configure tools`
* Some config parsing bugfixes for handling of accidentally saved `aerolab config defaults` values.
* Allow specifying not to configure TLS for `mesh` in `tls generate` command.
* Allow specifying multiple disks of same type in `gcp` backend; for example `--disk local-ssd@5` will request 5 local SSDs and `--disk pd-ssd:50@5` will request 5 `pd-ssd` of size 50GB.
* Improve feature key file version checking.
* Allow tagging by `owner` during cluster and client creation. Specify owner to always use with `aerolab config defaults -k '*.Owner' -v owner-name-all-lowercase-no-spaces`.
* Capture outcomes of run commands in internal user telemetry.
* Add telemetry version into output.
* Add `conf adjust` to allow for adjusting aerospike configuration on a deployed cluster on the fly.
* Move to using `Makefiles` for build and linux packaging process.
* Fix handling of extra docker flags in docker backend.

#### 6.1.0
* Add extra version check in `client tools` creator.
* Fix support for `amazon:2` on `arm` discovery.
* Add option for `logs get` to download logs in parallel using the `--threads X` option.
* Symlink `attach shell` command to `cluster attach` command.
* Add option in `tls generate` to have different bit sizes.
* If `-f` force option is not specified in the `destroy` commands, ask for confirmation via `stdin`.
* Basic anonymous telemetry enabled by default for internal Aerospike users.
* Added `inventory list` command, which prints all of the inventory (clusters, clients, firewalls, templates) in table or json format.
* Standardize the `list` commands to the `insventory list` output for cloud backends.
* Added `nano` command to all distributions.
* Update API packages to latest as of June 28th, 2023.
* Add new command `inventory instance-types` to allow for quick lookup of instance types in `AWS` and `GCP` clouds.
* Move all aerolab files and config paths to `~/.aerolab`.
* When creating firewalls/security groups, by default these will be locked to the caller's IP address.
* Allow to specify custom name for firewalls and security groups.
* Make the `GCP` backend disks use the `NVME` interface for disks.
* The `client create none` now supports backends other than Docker.

#### 6.0.3
* Handle bug in IP discovery on `docker` backend with custom networks.

#### 6.0.2
* Require GCP project when setting backend.
* Add handling for GCP local SSDs

#### 6.0.1
* Support for `gcp` backend.
* Add node exporter to the prometheus exporter setup.
* If feature file is not provided and cluster size is larger than 1, bail with critical error.
* Add `legacy` option to files `upload` and `download` functions.
* Partitioner `conf` command now requires a namespace before continuing.
* The `net block` command's speed greatly improved - changed execution logic.
* Allow to disable colors in `conf generate` command for some terminals.
* Add support for centos 9.
* Move to official `quay` repositories for centos version 8 and above.
* Move from centos 8 onwards to the stream edition.
* Multiple functional and OS-support bugfixes.
* Add `client create none` option for creating vanilla instances and containers.
* Check and confirm if docker is in use and clients are being deployed without forwarded ports.
* Deprecated `jupyter` client.

#### 5.4.6
* update all libraries (dependencies)
* aws `lock-security-groups` and `create-security-groups` now allows for connectivity from outside to the servers too

#### 5.4.5
* Fix duplicate switch problem when creating clusters.
* Add support for specifying a directory for feature file path to allow multiple feature file versions.

#### 5.4.4
* Try to adjust `vm.max_map_count=262144` on each client start for `elasticsearch` needs.
* Make `promtail` stack optional when launching `run_asbench` on the client machines.

#### 5.4.3
* handle malformed output from `asinfo` on error
* add backend option to specify non-standard temporary directory
* fix ulimits for linux docker desktop
* add auto-discovery of `WSL2` for temp dir creation

#### 5.4.2
* add `nodes` switch to `add exporter` command
* send a reload command on `configure ams` instead of restarting the service
* disable service update prompts for `apt` on `ubuntu:22.04` in AWS

#### 5.4.1
* add default `doc-id` source `digest`
* `cluster create` will now treat paths containing `.` in `aerospike.conf` as containing a file name for the purpose of precreating directories
* Updated asbench dashboards based on user testing, improving labels, and no longer averaging percentiles.

#### 5.4.0
* add basic latency graphs into `asbench` custom dashboards in `AMS`
* remove redundant declarations for item slices
* docker fix cluster listing with json format to ignore non-aerolab containers
* fix username/password switches for rest gateway client
* fix `ams` client installation of grafana on docker
* fix bug where `--detach` in client attach would run each command twice
* pass the `NODE` environment variable in all remote run commands - set to the aerolab node number inside the machine
* update dependencies:
  * `github.com/aerospike/aerospike-client-go/v6` version `6.9.1` to `6.10.0`
  * `github.com/aws/aws-sdk-go` version `v1.44.192` to `v1.44.214`
  * `golang.org/x/crypto` version `v0.5.0` to `v0.6.0`
  * `golang.org/x/term` version `v0.4.0` to `v0.5.0`
  * `github.com/rivo/uniseg` version `v0.4.3` to `v0.4.4`
  * `golang.org/x/sys` version `v0.4.0` to `v0.5.0`

#### 5.3.0
* from now on, `CGO` is disabled forcefully during build for portability
* build process for embedding arm/amd linux binaries now uses `go:embed` instead of base64-encoded strings
* rest gateway client added

#### 5.2.0
* `list-subnets` and `list-security-groups` will show more info in a cleaner (tabular) format
* added option to specify custom `astools.conf` file in `cluster create` and `client create tools` commands
* added `elasticsearch` connector to the `client create` options list
* added `connector`switch in `xdr connect` command

#### 5.1.1
* handle rack-id settings with roster in sc mode

#### 5.1.0
* add option `aerolab config docker` to handle multiple docker networks
* add option to specify network name when creating docker clusters/clients
* add interactive option - when a network doesn't exist, ask if one should be created
* add `aerolab conf rackid` option to add/change rack-id configuration on cluster nodes

#### 5.0.0
* add `SIGINT` handler to aerospike installer downloader, and template creators, to abort smoothly
* allow installation of FIPS edition of Aerospike
* auto-create default VPC if it's missing
* handle per-VPC aerolab-managed security groups (breaking feature - old security groups from `4.5.0` will no longer be used)
* new feature: `aerolab cluster partition` to allow for automated partitioner with automatic adjustment of `aerospike.conf` as required
* new `aerolab config aws` commands: `create-security-groups`, `list-security-groups`, `list-subnets`
* node expander bug fix - handle multiple clusters separated by commas in commands
* updating dependencies
  * golang `1.20`
  * golang build tools
  * github.com/aerospike/aerospike-client-go/v5 v5.10.0 => v5.11.0
  * github.com/aerospike/aerospike-client-go/v6 v6.6.0 => v6.9.1
  * github.com/aws/aws-sdk-go v1.44.158 => v1.44.192
  * github.com/mattn/go-runewidth v0.0.9 => v0.0.14
  * github.com/rivo/uniseg v0.4.3
  * github.com/yuin/gopher-lua v0.0.0-20220504180219-658193537a64 => v1.1.0
  * golang.org/x/crypto v0.3.0 => v0.5.0
  * golang.org/x/sys v0.2.0 => v0.4.0
  * golang.org/x/term v0.2.0 => v0.4.0

#### 4.6.0
* add client-side grafana dashboards to AMS monitoring stack client
* add `fullstack` documentation on how to deploy the full stack
* make cluster list better looking in docker backend
* add basic `aerospike.conf` config file generator UI in `aerolab conf generate` feature
* add `aerolab config aws lock-security-groups` for locking down sec groups to just the caller IP (for vscode, jupyter, AMS and optionally port 22)

#### 4.5.0
* fix bug with old Aerospike version download, requiring username/password
* add notes at the end of AMS and exporter installation, referencing one-another to avoid confusion
* allow for aerolab to auto-create security groups in aws, as well as discover subnet IDs based on AZ for the default VPC
* allow tagging instances and images with custom tags when using the aws backend
* add debian to the list of supported OS types

#### 4.4.4
* fix issue with the `autoload` directory execution in AWS
* add cluster existence check in `expandNodes` to handle `-l all` on non-existent cluster name

#### 4.4.3
* add cluster name checking and hostname validation on setting hostnames
* improve failed template vacuum (only delete the failed template container, not all template containers)
* fix aws missing-region discovery
* when parsing aerospike.conf, ignore commented out lines while parsing directory paths
* allow specifying multiple security group IDs in AWS
* change macos pkg install location to `/usr/bin/` instead of `/usr/local/bin/` due to missing `$PATH` on some installations

#### 4.4.2
* handle AWS library bug regarding unset regions

#### 4.4.1
* vscode client change installation order - java hangs otherwise on slow connections

#### 4.4.0
* improve handling of keypairs in AWS, especially templating ones
* aws: add auto-discovery of instance type to whether it is arm (obsolete `--arm` switch)

#### 4.3.10
* fix aws backend template image naming

#### 4.3.9
* aws backend arm template creation bugfix
* aws backend add arm/amd arch tracking to template names in `DeployTemplate` and `DeployCluster`
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
* ams exporter fix AWS installation
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
* experimental `jupyter` client added with `go,python,java,node,dotnet` kernels and Aerospike client libraries
* experimental `trino` client added
* fix `net loss-delay` in source being client bug
* adjust installer downloader to new 6.2 Aerospike naming convention

#### 4.3.3
* bump version of all dependencies to latest
  * resolves a number of known issues in golang/x/crypto, golang/x/term and golang/x/sys

#### 4.3.2
* add support for pre-selected eu-central-1
* add ability for AWS backend to automatically lookup AMIs for any region using DescribeImages
* add `rest-api` command to allow for using AeroLab as a rest-api webserver (basic, not full rest-api) instead of cli interface
* error handling improvements
* minor flow bugfixes
* package `aerolab` as `pkg` for MacOS, `deb` for debian/ubuntu, `rpm` for rhel/centos and generic `zip` for linux

#### 4.3.1
* update Aerospike client libraries to latest versions

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
  * `cluster start` with multiple clusters would not fix mesh config properly, nor start Aerospike

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
  * add options to deploy client machines (AeroLab client, AeroLab attach client, backend support for server/client selector, files command support, TLS command support)
    * clients: base, aerospike-tools
  * add client command documentation
* Improvements
  * `aerolab xdr connect` command: add support for cross-region AWS backend

#### v4.0.2
* add nodeExpander:
  * (node list can now be: 1-10,15,-3 - i.e. 1 to 10, node 15, not node 3)
  * (or: ALL,-5 - i.e. all nodes except node 5)
* bugfix in ranges in `files sync`
* fix multiple completion bugs
* add useful print of config for TLS generate command

#### v4.0.1
* add support for CentOS / RHEL-based distros v 7 and 8 in aws
* add option to rename hostname of nodes to clusterName-nodeNo
* cleanup command line interface (changes cli usage)
* make-cluster distro version help page: remove CentOS 6 and add CentOS 8 and Ubuntu 22.04 to supported values
* use submodules for common functions and methods
* remove support for CentOS 6 / RHEL 6
* add 'edit' command line option for quick-editing a single file
* add 'sync' option for quick-syncing of files/directories from one node to other nodes
* make 'download' and 'upload' work on whole directories, recursively, not just files
* update readme and help pages
* update scripts/
* make help pages work without verifying if backend is working
* install a script to run before Aerospike starts and after it stops
* add basic troubleshooting tools to templates
* add json output support to template and cluster list
* bash completion and `zsh` completion

#### v3.1.2
* `make-cluster` and `cluster-grow`: automatically add `cluster-name` to `aerospike.conf` unless specified not to by the `-O` switch
* remove dependency on obsolete `ioutil` package
* bump to latest `golang` version for compile
* cgo minimum macOS version is locked during build
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
* store deployed Aerospike version in `/opt/aerolab.aerospike.version`
* deprecate `-5` switch for version selection in `xdr-connect`
* add `xdr-version` selector in `xdr-connect`, add `auto` option for auto-discovery
* add `restart-source` option in `xdr-connect` with default of 'yes' to allow for auto-restarting of source on XDR static configuration
* bring version discovery features from `make-cluster` into `upgrade-aerospike`

#### v3.0.4
* documentation cleanup
* fix version discovery mechanism
* add option for specifying partial version, for example `-v '4.9.0.*'` will find and use latest `4.9.0.` version
* added command `list-versions` to quickly lookup Aerospike versions, with switches for easy sorting and filtering, see `list-versions help` for more details

#### v3.0.3
* bugfix: AWS backend using ubuntu 20.04 image correctly now
* AWS backend make "waiting for node to come up" messages more explicit

#### v3.0.2
* add option in insert-data and delete-data to choose Aerospike library version (4|5)

#### v3.0.1
* deploy-container move to ubuntu 20.04
* add basic tools to Aerospike server containers
* set basic ubuntu version to 20.04
* update version discovery algorithm to allow for new naming conventions
* add AWS backend ubuntu 20.04 discovery options
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
* recompile with Aerospike library v5

#### 2.66
* fix check in AWS backend for public IP

#### 2.65
* fix TLS - new requirements - cannot use Common Name any more

#### 2.64
* new Aerospike website broke artifacts download links. This works around the problem.

#### 2.63
* satisfy libcurl4 dependency for asd 5.1+

#### 2.62
* improvement: will now check if instance in AWS has public IP assigned before attempting to use the variable

#### 2.61
* insert-data now allows specifying to insert data only to X number of partitions and/or nodes, or to specify exact partition numbers to insert data to

#### 2.60
* fix clash in switches in net-loss-delay

#### 2.59
* error handling improvement: add handling of wrong docker image names
* updated templates to all have default-ttl 0
* updated dependencies to latest version

#### 2.58
* fix support for running AeroLab via symlinks or from a PATH env var

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
* added 'latest' recognition fix for Aerospike version 5
* insert-data multithread switch fix
* add OS discovery for aerospike 5+
* fix links in AeroLab downloads
* NEW: xdr-connect supports xdr in asd v5+, using the '-1' switch

#### 2.48
* fixed -u switch in insert-data

#### 2.47
* fixed setting correct writepolicy on Aerospike insertData

#### 2.46
* minor bugfixes
* shrunk binary size to 12MB from 56MB

#### 2.45
* AWS BACKEND FIX - small bug when dealing with IP address of nodes

#### 2.44
* attempt to install python3-setuptools as well as python3-distutils - because ubuntu loves making changes
* updated dependency versions and aerospike library to latest

#### 2.43
* added message on conf-fix-mesh to inform user that Aerospike must be restarted
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
* fix critical bug with CentOS deployments

#### 2.35
* fix critical bug in ubuntu deployment

#### 2.34
* OPS-3268 - fixed auto-discovery of versions after ordering change in artifact webserver

#### 2.33
* fix for python3-distutils dependency in old ubuntu versions

#### 2.32
* large number of small typo fixes for messaging and error reporting
* OPS-3222 - fix naming in help pages
* OPS-3237 - provide warning if Aerospike 4.6+ is used and feature file not provided

#### 2.31
* fixed never-ending template builds for CentOS 7 - disabled firewall control (net-loss and rate control) on centos builds

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
* Aerospike Python client fails on ubuntu 18.04 with 'undefined symbol: OPENSSL_sk_num' error. Downgraded all client libraries to use 16.04

#### 2.17
* insert-data fixed reporting bug which would report error even if no error was present if the insert was too fast
* insert-data some errors went unreported and were treated as success, fixed

#### 2.16
* get-logs should return .log file names
* cluster-list on docker now only prints containers associated with aerolab
* added ldap deployment script and documentation on how to configure aerospike to deploy ldap
* fixed \r location on download progress report

#### 2.15
* AWS backend: make-client, net-block, net-unblock, net-list

#### 2.14
* AWS backend support - experimental, feature yet not implemented: make-client, net-block, net-unblock, net-list
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
* In docker, auto-fix log location of Aerospike log in aerospike.conf (no journalctl)
* If target (remote or local) is Darwin, use docker default, otherwise use LXC default
* Fixed Centos creation never finishes for template on CentOS 7
* Fixed custom startup script for Aerospike in CentOS 7 on docker (damn you, docker!)

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
