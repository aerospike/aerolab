# Deploying Python Clients

This script allows for quick and easy deployment of a `python` client library with sample code in a docker container.

This can be used on it's own, as part of [aerolab-buildenv](../aerolab-buildenv/README.md) script or combined with the `aerolab` command.

## Deploying

### First clone this repo

#### Using https

```bash
git clone https://github.com/citrusleaf/aerolab.git
```

#### Using git keys

```bash
git clone git@github.com:citrusleaf/aerolab.git
```

### Enter this directory

```bash
cd aerolab/scripts/aerolab-pythonclient
```

### Get usage help

```bash
% ./runme.sh
```

### Create new GoClient container

```bash
% ./runme.sh run
```

### Attach to container shell

```bash
% ./runme.sh attach
$ ls
```

### Destroy

```bash
% ./runme.sh destroy
```

## Usage

```bash
% ./runme.sh 

Usage: ./runme.sh start|stop|destroy|run|get

  run     - create and start Client Node
  start   - start an existing, stopped, Client Node
  stop    - stop a running Client Node, without destroying it
  get     - get the IPs of Client Node
  attach  - attach to the client container
  destroy - stop and destroy the Client Node
```

## Code Samples

Once you have attached to the container, you will see the client example code in /root/clients.

There are 3 example pieces of code :-

Name | Description
--- | ---
example_basic.py | This does a basic database connect, writes a record and reads it back
example_auth.py | This does an authenticated database connect, writes a record and reads it back.
example_tls.py | This does an authenticated database connect over TLS, writes a record and reads it back.
