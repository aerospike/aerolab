# TODO

## Next - MVP

CODE:
* do a documentation - dictate cool bells and whistles (code cleanup, listing items with json, table, etc, and all others)
* all missing commands

CONSIDER:
* test disk caching of inventory (once we have commands so we can actually test it)
* on backend initialization, we do not always want to do a full inventory at startup as we may be calling Execute from another Execute? Or will we?
* do we want pagination at all for inventory listings? One would assume that not, as people know how to use `less`
* consider how to solve the aerolab-embedded-in-aerolab problem better

OTHER:
* RELEASE.md
* README.md
* docs/*
* src/Makefile
* .github
* `aerolab migrate` - migrate the config directory to the new format (ssh keys, etc) AND the Instances in AWS/GCP. Warn Docker cannot be migrated.

## Notes

### Software configurators

* when logging - include version of every installed software
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
