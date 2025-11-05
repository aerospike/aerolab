# TODO

## Next - MVP

CODE:
* make `cloud databases list` and `cloud databases credentials list` and `cloud secrets list` also return friendly tables instead of json
* add `cloud databases get tls-cert` and `cloud databases get host` commands to get connection details for the DB
* add option for `cloud databases wait-for-status` to wait for a specific health.status for a specific DB ID - check every 10 seconds, with optional wait-timeout
* review all comands to ensure only the ones that need to are invalidating the inventory caches
* xdr - manage xdr configuration on running instances
* tls - manage tls certificates on running instances
* client - wrapper around instances command + templates + their own software installation and configuration
* net
* data
* `aerolab migrate` - migrate not only configs, but also VMs
* testing
* documentation
* PRERELEASE
* add support for docker amazonlinux:2023
* agi, web, rest
* PRERELEASE

OTHER:
* All cmd package TODOs in code
* instance-types backend in AWS is unable to pull prices for metal instances (probably it's under something other than `on-demand` or `spot`)
* instance-types backend in GCP cannot pull some instances - notably ct5l and c2 types as well as some m_ types, x_ types and a4
* test using custom images with `instances create` on all backends
* aerolab if failed grow/create/apply on capacity, retry automatically
* test disk caching of inventory (once we have commands so we can actually test it)
* review all defaults
* review when and how to retry failures related to work, especially the create commands
* review v7.8+ fixes and ensure those features are in v8

## Notes

* Instance auto-sizing (or just sizing)
* Action notifications
* Retries/progress and continue from last point

V7 missing file list:
* RELEASE.md
* README.md
* docs/*
* src/Makefile
* .github
