[Docs home](../../README.md)

# Quick build client - Python


This script allows for quick and easy deployment of a Python-based client
library with sample code in a Docker container.

It can be used on its own, as part of an [aerolab-buildenv](/tools/aerolab/utility_scripts/ldap-tls)
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
cd aerolab/scripts/aerolab-pythonclient
```

## Get usage help

```bash
./runme.sh
```

### Create a new Python client container

```bash
./runme.sh run
```

### Attach to the client container's shell

```bash
./runme.sh attach
ls
```

## Destroy the client container

```bash
./runme.sh destroy
```

## Usage

```bash
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

Once you have attached to the container, you will see the client example code in `/root/clients`.

There are 3 example code snippets:

Name | Description
--- | ---
example_basic.py | Does a basic database connect, writes a record and reads it back.
example_auth.py | Does an authenticated database connect, writes a record and reads it back.
example_tls.py | Does an authenticated database connect over TLS, writes a record and reads it back.
