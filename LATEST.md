# [v7.4.0](https://github.com/aerospike/aerolab/releases/tag/7.4.0)

_Release Date: UNDEF_

**Release Notes:**
* FEATURE: Web UI.
* FRATURE: DOCKER: Add support for multiarch. See [this page](https://github.com/aerospike/aerolab/tree/master/docs/docker_multiarch.md) for details.
* FIX: GCP: Many commands would fail during template creation, making parallel use imposible. Fixed.
* FIX: GCP: Delete `arm` templates was not working at all.
* FIX: The `net list` command does not work when client has same name as server.
* FIX: The `net loss-delay` feature would fail to activate a python environment.
* ENHANCEMENT: Support all ubuntu 18+ and centos 7+ builds with `net loss-delay` feature.
* ENHANCEMENT: When `--on-destination` is selected in `net loss-delay`, set `--src-network` instead of `--network`.
* ENHANCEMENT: The `net loss-delay` feature now supports specifying ports.
* ENHANCEMENT: The `logs get` command will append original filename as suffix and will ask if files would be overwritten unless `-f` is specified.
* ENHANCEMENT: For centos stream 8/9 installs, there is no more need to re-enable repos and sync distros.
* ENHANCEMENT: All inventory instance listings in cloud will now show instance type in the last field.
* ENHANCEMENT: Tested and documented podman backend support.
