# [v7.5.0](https://github.com/aerospike/aerolab/releases/tag/7.5.0)

_Release Date: Month day, year_

**Release Notes:**
* FEATURE: Added new client: `vector`.
* FEATURE: Added new client: `graph`.
* ENHANCEMENT: Added option to add specific ports to the firewall rules on GCP and AWS backends.
* ENHANCEMENT: AGI will not parse and graph `security.c` type messages for authentication message counts per second.
* ENHANCEMENT: WebUI add option to `attach trino` for the trino client cli.
* ENHANCEMENT: GCP Expiry system will attempt to enable required services prior to installation.
* ENHANCEMENT: Docker: for version 7+ of Aerospike, if a custom config file is not provided, modify the default one to remove `bar` namespace and change `test` to use file-backed `storage-engine device`.
* FIX: AGI: Ingest `aggregate` would incorrectly miss aggregation values when working with lines that contain `(repeated:X)`.
* FIX: AGI: Handle connection error during installation gracefully.
* FIX: WebUI: make listener bind specifically to IPv4 or IPv6 depending on specified address instead of letting the kernel decide.
* FIX: WebUI: do not clear running jobs from history in the web interface until they are completed (when clear history button is pressed).
