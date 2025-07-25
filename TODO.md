# TODO

## Next - MVP

* src/cli
  * plus have CLI outputs in 2 formats: json, text; logging and telemetry should be in json only
  * will need logger to handle that
  * use error types, like {"Result": "ERROR", "Error": "GCP_FAILURE", "Message": "GCP failed to create instance"}
  * when logging - include version of every installed software
* RELEASE.md
* README.md
* docs/*
* src/Makefile
* .github
* telemetry
* `aerolab migrate`

## Notes

### Software configurators

* when aerolab starts aerospike, we do do not get cake, but error, maybe we should grab the config and log that it didn't start
* maybe builtin openvpn container deployment via aerolab? Or tailscale? Or SOMETHING!
* Aerospike server `aerospike.conf`, xdr conf between clusters
* Aerospike server live - reroster, xdr, etc
* Aerospike tools `astools.conf`
* Aerospike exporter `ape.toml`
* Aerospike Backup Service
* Grafana
* Aerolab
* Prometheus
* Vscode Extensions for common languages
* EksCtl
* Goproxy
* easytc
* AeroLab AGI
  * plugin
  * inject
  * proxy
  * grafana-fix
* nftables rule manager

### Other

* Instance auto-sizing (or just sizing)
* Telemetry
* Action notifications
* Retries/progress and continue from last point
* Self upgrader
* Showcommands - showsysinfo, etc
* Aerospike data insert tool
* TLS Generators

### Compatibility

* aerolab migrate on backend side

### CLI porting

* Port existing CLI to use new hooks - CLI is to be dumb
* Write non-CLI logic functions which handle logic of calling backends, software installers, etc, that CLI simply calls

### WebUI

* Should just work if the CLI is ported like-for-like
* Redo rest-api
