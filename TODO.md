# TODO

## Software installers/downloaders

* Grafana
* Aerolab
* Prometheus
* Vscode App
* EksCtl
* Golang, python3 (+pip), java, dotnet, gcc, make (build-essential+)

## Software configurators

* Aerospike server `aerospike.conf`, xdr conf between clusters
* Aerospike server live - reroster, xdr, etc
* Aerospike tools `astools.cong`
* Aerospike exporter `ape.toml`
* Aerospike Backup Service
* Grafana
* Aerolab
* Prometheus
* Vscode Extensions for common languages
* EksCtl
* AeroLab AGI
  * plugin
  * inject
  * proxy
  * grafana-fix

# Linux tools

* iptables
* iproute2 (easytc)

## Other

* Instance auto-sizing (or just sizing)
* Telemetry
* Action notifications
* Retries/progress and continue from last point
* Self upgrader
* Showcommands - showsysinfo, etc
* Aerospike data insert tool
* TLS Generators

# Compatibility

* aerolab migrate on backend side

## CLI porting

* Port existing CLI to use new hooks - CLI is to be dumb
* Write non-CLI logic functions which handle logic of calling backends, software installers, etc, that CLI simply calls

## WebUI

* Should just work if the CLI is ported like-for-like
* Redo rest-api
