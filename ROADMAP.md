# Roadmap

## Planned

### 6.3

* Add option for auto-expiring clusters in GCP and AWS
* Add option to map and expose ports in Docker backend on a 1:1 pairing (eg 3100 node 1, 3101 node 2 etc) - so not workarounds are needed to access Aerospike clusters on Docker Desktop from the Desktop
* AeroLab Support for deploying Amazon 2023 server >= 6.4
* AeroLab Support for deploying Debian 12 server >= 6.4

### 6.4

* Add AGI (Aerospike-Grafana-Integrated stack) for graphing Aerospike log statistics in Grafana.

## Other

* Support for `Azure` backend.
* Clients/Connectors - make connectors more plugin-based using deployment scripts instead of baking in deployments into aerolab.
* Consider adding telemetry to all users (anonymous usage only).
* Consider adding option of `ansible` or other backend (gen scripts instead of performing deployments).
* Consider tailscale integration.
* support multi-docker installs
* support golang templates for automation of client installation
