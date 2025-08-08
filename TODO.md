# TODO

## Next - MVP

CODE:
* Should we give people the ability to specify custom images via image name or ID for AWS/GCP?
* `instances apply` - create, grow, destroy - apply a state - inform user what it destroys - with interactive mode
* volume - manage volumes on running instances
* `inventory list` - add volumes list
* subcommands:
  * `inventory ansible`
  * `inventory genders`
  * `inventory hostfile`
* files - upload/download files to/from instances
* attach - wrapper around instances attach, plus extra functionality
* cluster, client - wrapper around instances command + templates + their own software installation and configuration
* conf - manager aerospike configuration on running instances
* aerospike - start/stop/restart/status of aerospike on running instances
* logs - download logs from running instances
* roster - manage strong-consistency rosters on running instances
* xdr - manage xdr configuration on running instances
* Others to code after:
  * tls, net, data, agi
  * web, rest
* instance-types backend in AWS is unable to pull prices for metal instances (probably it's under something other than `on-demand` or `spot`)
* instance-types backend in GCP cannot pull some instances - notably ct5l and c2 types as well as some m_ types, x_ types and a4

CONSIDER:
* test disk caching of inventory (once we have commands so we can actually test it)
* consider how to solve the aerolab-embedded-in-aerolab problem better
* in every command, if Interactive, for missing option items, do NOT error, instead ask for entry (AskForString, AskForInt, AskForFloat, choice.Choice)
  * print out constructed new command line

OTHER:
* RELEASE.md
* README.md
* docs/*
* src/Makefile
* .github
* `aerolab migrate` - migrate the config directory to the new format (ssh keys, etc) AND the Instances in AWS/GCP. Warn Docker cannot be migrated.
  * support --from HOME --to HOME, or if not specified, migrate from `~/.aerolab` to `~/.config/aerolab`

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
