# [v7.5.0](https://github.com/aerospike/aerolab/releases/tag/7.5.0)

_Release Date: Month day, year_

**Release Notes:**
* FEATURE: Added new client: `vector`.
* FEATURE: Added new client: `graph`.
* ENHANCEMENT: Added option to add specific ports to the firewall rules on GCP and AWS backends.
* ENHANCEMENT: AGI will not parse and graph `security.c` type messages for authentication message counts per second.
* ENHANCEMENT: WebUI add option to `attach trino` for the trino client cli.
* ENHANCEMENT: GCP Expiry system will attempt to enable required services prior to installation.
* FIX: AGI: Ingest `aggregate` would incorrectly miss aggregation values when working with lines that contain `(repeated:X)`.
