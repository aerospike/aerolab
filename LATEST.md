# [v7.3.0](https://github.com/aerospike/aerolab/releases/tag/7.3.0)

_Release Date: UNDEF_

**Release Notes:**
* Volume command supports mounts on centos/amazon.
* Version check logic - if user is using a dev build and a final build is out, inform user of available upgrade.
* Allow choosing renderer mode for inventory listing to be normal, TSV, CSV, Markdown or HTML.
* Tag expiry systems to allow for version discovery and automated updating.
* Add `aerolab upgrade [--edge]` command to allow upgrading to latest stable (or latest pre-release).
* Windows: catch the use of command prompt and warn against it.
* Improvement: replace the `mesh/multicast` fix system with `aeroconf` parser.
* Improvement: in `aerolab conf namespace-memory`, adjust `data-size` instead if using aerospike `v7+`, and only if required. Otherwise `noop`.
* Allow adding of `aerolab` to a server/client instance on-demand.
* AGI commands support centos/amazon.
* AGI: support v7 of aerospike.
* AGI: support arm editions of centos-based operating systems.
* AGI: add `--no-dim-filesize` option to specify data storage file size for non-data-in-memory namespaces.
* AGI: override tools package by default by the latest tools. Allow `--no-tools-override` to disable.
* AGI: All stack items utilize `v7` aerospike client library.
* AGI: Feature: Monitor agi instance states and react accordingly - sizing or cycling from spot to on-demand types.
* Docker: Improvement: attempt to auto-adjust limit of open files.
* Docker: when exposing multiple ports in a continuous order, only use first exposed port for the service port.
* GCP: The `cluster add firewall` command should issue `op.Wait` after all operations have been queued.
* GCP AGI spot instance support.
* GCP Volumes support (using standard `pd-ssd`, and requiring size, as there is no `EFS` equivalent).
* AWS: Always encrypt EBS volumes.
* Fix: `conf generate` command for v7 aerospike is missing an argument `data-size` where needed.
* Fix: add support for `time.Duration` to `config defaults` command.
* Fix: `config defaults` should be able to set defaults for any parameter, even if it is for a backend not currently in use.
* Fix: aws security groups will now also open ICMP traffic in the rules.
* Fix: GCP: for some instances, `TERMINATE` scheduling policy must be set.
* Fix: Docker show IPs for containers which do not have exposed ports.
* Fix: AMS client fix regression: `asbench` dashboard installation.
* Fix: Auto-create temporary directory as needed, if it is specified.
* Fix: GCP: Handle firewall rules which use an IP instead of CIDR.
* Fix: New exporter name changes cause 1.15.0 to not install. This uses the new names.
* Fix: `lsblk` now shows sizes as float not int.
* Aerospike Client Versions: `data insert`, `data delete` support for `v7` aerospike client.
