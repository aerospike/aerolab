[Docs home](../../README.md)

# Deploying an LDAP server


This script set allows for easy deployment of an LDAP server with or without TLS,
and LDAP admin web UI in docker containers.

This script can be used on its own, or as part of an [aerolab-buildenv](/tools/aerolab/utility_scripts/ldap-tls)
script, or in combination with `aerolab` commands.

## Usage

```
./runme.sh

Usage: ./runme.sh start|stop|destroy|run|get

  run     - create and start LDAP stack
  start   - start an existing, stopped, LDAP stack
  stop    - stop a running LDAP stack, without destroying it
  get     - get the IPs of LDAP stack
  help    - get a list of useful commands for cli ldapsearch
  destroy - stop and destroy the LDAP stack
```

## Getting started

### Clone the repo

```bash
git clone https://github.com/aerospike/aerolab.git
```

### Enter the directory

```bash
cd aerolab/scripts/aerolab-ldap
```

### Get usage help

```bash
./runme.sh
```

### Run new LDAP server with LDAP admin

```bash
./runme.sh run
```

### Destroy

```bash
./runme.sh destroy
```

## Notes

  * All certificates are in the `certs/` directory.
  * This LDAP supports both ldap:// and ldaps:// (SSL) out of the box.
  * Hostname and CN for the certificate for the ldap server is `ldap1`.
  * This also deploys the ldapadmin web GUI for web administration (create/delete groups/users).
  * At the end of `runme.sh run`, a useful list of commands and IPs is printed to access the ldap and web UI.
  * Run `runme.sh get` to get the useful list again.
  * When configuring LDAP on the Aerospike side, specify 
    either `ldap://ldap1:389` or `ldaps://ldap1:636` in the LDAP server name.
  * You then need to add the `ldap1` host pointing at the IP of the LDAP server in the `/etc/hosts` file.
    This is because Aerospike will only be able to connect and verify if the hostname of the
    LDAP server matches the CN of certificate the LDAP server uses, which is `ldap1`.
  * Take note of `LDAP_TLS_VERIFY_CLIENT: try` in docker-compose.yml. If that is set to `demand`,
    the LDAP server requires mutual certificate authentication with the Aerospike server. The server will need
    a proper certificate for that, not just the CA.

## Advanced

### Export/Import

From the ldap-admin UI you can export `ldif` files. You can then import those files by putting
your definitions in the `ldif/` directory. These will be automatically deployed when you do
the `run` command again.
