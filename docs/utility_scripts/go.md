[Docs home](../../README.md)

# Quick build client - Go


This script allows for quick and easy deployment of a Go client library with
sample code in a Docker container.

This script can be used on its own, as part of an [aerolab-buildenv](/tools/aerolab/utility_scripts/ldap-tls)
script, or combined with the `aerolab` command.

## Clone the repo

### Using https

```bash
git clone https://github.com/aerospike/aerolab.git
```

### Using git keys

```bash
git clone git@github.com:aerospike/aerolab.git
```

## Enter the directory

```bash
cd aerolab/scripts/aerolab-goclient
```

## Get usage help

```
./runme.sh
```

## Create a new Go client container

```bash
./runme.sh run
```

### Attach to the client container's shell

```
./runme.sh attach
ls
```

## Destroy the client container

```bash
./runme.sh destroy
```

## Usage

```
./runme.sh 

Usage: ./runme.sh start|stop|destroy|run|get

  run     - create and start Client Node
  start   - start an existing, stopped, Client Node
  stop    - stop a running Client Node, without destroying it
  get     - get the IPs of Client Node
  attach  - attach to the client container
  destroy - stop and destroy the Client Node
```

## Code samples

After you attach to the container, the samples are in the following locations:

* `/root/go/src/v4` for the version 4 library
* `/root/go/src/v5` for the version 5 library

Each directory contains the following examples:

Name | Description
--- | ---
aerospike-basic | Aerospike basic connect, plus some get/put command examples.
aerospike-auth | Aerospike connect with basic authentication, plus some get/put command examples.
aerospike-tls | Aerospike connect with TLS and external LDAP authentication, plus some get/put command examples.
