# Dev scripts

Name | Description
--- | ---
new-expiry-version.sh | when changing the expiry version code, run this script to bump the expiry version number in aerolab; this will ensure aerolab upgrades expiry systems to this version; this will also recompile the expiry system
new-telemetry-version.sh | when changing the telemetry version code, run this script to bump the telemetry version number in aerolab; this will send the new telemetry version to the cloud function
upgrade-deps.sh | go to all relevant directories, and update all deps
new-agi-version.sh | when changing the agi version code, run this script to bump the agi version number in aerolab; this will ensure aerolab upgrades agi templates to this version