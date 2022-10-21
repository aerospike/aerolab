# Deploying Go Clients

This script allows for quick and easy deployment of a `go` client library with sample code in a docker container.

This can be used on it's own, as part of [aerolab-buildenv](../aerolab-buildenv/README.md) script or combined with the `aerolab` command.

## Deploying

### First clone this repo

#### Using https

```bash
git clone https://github.com/aerospike/aerolab.git
```

#### Using git keys

```bash
git clone git@github.com:aerospike/aerolab.git
```

### Enter this directory

```bash
cd aerolab/scripts/aerolab-goclient
```

### Get usage help

```
% ./runme.sh
```

### Create new GoClient container

```
% ./runme.sh run
```

### Attach to container shell

```
% ./runme.sh attach
$ ls
```

### Destroy

```
% ./runme.sh destroy
```

## Usage

```
% ./runme.sh 

Usage: ./runme.sh start|stop|destroy|run|get

  run     - create and start Client Node
  start   - start an existing, stopped, Client Node
  stop    - stop a running Client Node, without destroying it
  get     - get the IPs of Client Node
  attach  - attach to the client container
  destroy - stop and destroy the Client Node
```

## Sample code

Once you attach to the container, you will find the samples in the following location:

* `/root/go/src/v4` for the version 4 library
* `/root/go/src/v5` for the version 5 library

In each directory you will find the following examples:

Name | Description
--- | ---
aerospike-basic | Aerospike basic connect, plus some get/put command examples
aerospike-auth | Aerospike connect with basic authentication, plus some get/put command examples
aerospike-tls | Aerospike connect with TLS and external ldap authentication, plus some get/put command examples
