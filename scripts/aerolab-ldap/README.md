# Deploying an LDAP server

This script set allows for easy deployment of an LDAP server with or without TLS, and LDAP admin web UI in docker containers.

This can be used on it's own, with the [aerolab-buildenv](../aerolab-buildenv/README.md) script, or in combination with AeroLab commands.

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

### First clone this repo

```bash
git clone https://github.com/aerospike/aerolab.git
```

### Enter this directory

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

  * all certificates are in the `certs/` directory
  * this LDAP supports both ldap:// and ldaps:// (ssl) out of the box
  * hostname and CN for the certificate for the ldap server is `ldap1`
  * this also deploys the ldapadmin web GUI for web administration (create/delete groups/users)
  * at the end of `runme.sh run`, a useful list of commands and IPs is printed to access the ldap and web UI
  * run `runme.sh get` to get the useful list again :)
  * when configuring LDAP on Aerospike side, in the LDAP server name specify either `ldap://ldap1:389` or `ldaps://ldap1:636`
  * you then need to add the `ldap1` host pointing at the IP of the LDAP server in the `/etc/hosts` file
  * this is because Aerospike will only be able to connect and verify IF the hostname of the LDAP server matches the CN of certificate the LDAP server uses; which is `ldap1`
  * take note of `LDAP_TLS_VERIFY_CLIENT: try` in docker-compose.yml; if that is set to `demand`, LDAP server requires mutual certificate auth with Aerospike server and server will need a proper certificate for that, not just the CA

## Advanced

### Export/Import

From ldap-admin UI you can export `ldif` files after settings things just right.

You can then import those by putting your definitions in the `ldif/` directory. These will be automatically deployed when you do the `run` command again.
