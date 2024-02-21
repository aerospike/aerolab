# [v7.4.0](https://github.com/aerospike/aerolab/releases/tag/7.4.0)

_Release Date: January 2, 2024_

**Release Notes:**
* FEATURE: Web UI.
* FRATURE: DOCKER: Add support for multiarch. See [this page](https://github.com/aerospike/aerolab/tree/master/docs/docker_multiarch.md) for details.
* FIX: GCP: Many commands would fail during template creation, making parallel use imposible. Fixed.
* FIX: GCP: Delete `arm` templates was not working at all.
* FIX: GCP: `client configure firewall` and `cluster add firewall` should not be locking the rules being added.
* FIX: GCP: Expiry system now stores information in a bucket to allow for multi-tenancy usage scenarios.
* FIX: The `net list` command does not work when client has same name as server.
* FIX: The `net loss-delay` feature would fail to activate a python environment.
* Fix: Support for `all` in `ClusterName` was broken when node expander was implemented.
* FIX: Docker: Regression - underscores in cluster names are allowed and should work.
* FIX: Docker: Issue with using new template naming conventions when old templates exist.
* FIX: AGI: AGI Commands would panic if cluster is not found.
* FIX: AGI: Default sftp threads to 1 and s3 threads to 4.
* FIX: AGI: Port 443 on AGI firewall in GCP should not lock to caller's IP by default.
* FIX: AGI: agi monitor would not work following adding of checks for sftp password.
* ENHANCEMENT: Owner tag, if not manually specified, will be filled with current OS username.
* ENHANCEMENT: Support all ubuntu 18+ and centos 7+ builds with `net loss-delay` feature.
* ENHANCEMENT: When `--on-destination` is selected in `net loss-delay`, set `--src-network` instead of `--network`.
* ENHANCEMENT: The `net loss-delay` feature now supports specifying ports.
* ENHANCEMENT: The `logs get` command will append original filename as suffix and will ask if files would be overwritten unless `-f` is specified.
* ENHANCEMENT: For centos stream 8/9 installs, there is no more need to re-enable repos and sync distros.
* ENHANCEMENT: All inventory instance listings in cloud will now show instance type in the last field.
* ENHANCEMENT: Tested and documented podman backend support.
* ENHANCEMENT: AGI: Check sftp access and file count in directory prior to creating anything.
* ENHANCEMENT: AGI: (gcp/aws) Always check if the selected instance type is large enough.
* ENHANCEMENT: AGI: Improve sftp source download speed by using sftp package concurrency.
* ENHANCEMENT: AGI: Change `write-block-size` to `8M` to allow for large `collectinfo` imports. Handle imports larger than 8M by trimming data.
* ENHANCEMENT: logging adds timezone now.
